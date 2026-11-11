(ns barn-checkpoint.core
  "Jepsen list-append workload against one disposable Barn server.

  Every client and nemesis operation is a JSON line sent to the lifecycle and
  MOO transport adapter in scripts/jepsen_checkpoint.py, which owns the Barn
  process generations. Each client request carries a run-unique operation id
  that the adapter journals and echoes; completions carry it as :op-id with
  the adapter's :generation and raw native response. The finished history is
  written as JSON lines; the adapter reconciles it with its journal, splits it
  per process generation and checks it with the pinned Elle CLI and the
  explicit checkpoint baselines."
  (:gen-class)
  (:require [clojure.data.json :as json]
            [clojure.java.io :as io]
            [clojure.tools.cli :as cli]
            [clojure.tools.logging :refer [info]]
            [elle.list-append :as la]
            [jepsen [checker :as checker]
                    [client :as client]
                    [core :as jepsen]
                    [generator :as gen]
                    [nemesis :as nemesis]
                    [random :as jrand]
                    [tests :as tests]])
  (:import (java.io BufferedReader BufferedWriter EOFException IOException
                    InputStreamReader OutputStreamWriter)
           (java.net InetSocketAddress Socket SocketTimeoutException)
           (java.util.concurrent.atomic AtomicLong)))

(defn adapter-connect
  "Opens one adapter connection. timeout-ms bounds each reply; the adapter
  answers every request itself, so this only fires if the adapter hangs."
  [port timeout-ms]
  (let [socket (Socket.)]
    (.connect socket (InetSocketAddress. "127.0.0.1" (int port)) 5000)
    (.setSoTimeout socket (int timeout-ms))
    {:socket socket
     :in     (BufferedReader. (InputStreamReader. (.getInputStream socket) "UTF-8"))
     :out    (BufferedWriter. (OutputStreamWriter. (.getOutputStream socket) "UTF-8"))}))

(defn adapter-close [conn]
  (when conn (.close ^Socket (:socket conn))))

(defn request!
  "Sends one request and returns the adapter's reply map."
  [conn message]
  (doto ^BufferedWriter (:out conn)
    (.write (str (json/write-str message) "\n"))
    (.flush))
  (let [line (.readLine ^BufferedReader (:in conn))]
    (when (nil? line)
      (throw (EOFException. "adapter closed the connection")))
    (json/read-str line :key-fn keyword)))

(defn decode-mop [[f k v]] [(keyword f) k v])

(defn complete
  "Applies an adapter reply to an invocation. Only :ok replaces the value; a
  :fail or :info keeps the invoked transaction."
  [op reply]
  (let [outcome (keyword (:type reply))]
    (cond-> (assoc op :type outcome
                      :generation (:generation reply)
                      :error (:error reply)
                      :raw (:raw reply))
      (= :ok outcome) (assoc :value (mapv decode-mop (:value reply))))))

(defrecord Client [port timeout-ms ids conn]
  client/Client
  (open! [this test node]
    (assoc this :conn (adapter-connect port timeout-ms)))
  (setup! [this test] this)
  (invoke! [this test op]
    ; ids is shared by every client of the run: the id names this operation
    ; in the adapter journal and in the completion (:op-id).
    (let [id (.incrementAndGet ^AtomicLong ids)
          op (assoc op :op-id id)]
      (try
        (let [reply (request! conn {:op "txn" :id id :process (:process op) :value (:value op)})]
          (when (not= id (:id reply))
            (throw (IOException. (str "adapter reply for operation " (:id reply)
                                      " arrived for operation " id))))
          (complete op reply))
        ; The adapter may have sent the transaction; the outcome is unknown.
        (catch SocketTimeoutException e
          (assoc op :type :info :error {:kind "adapter-timeout"}))
        (catch IOException e
          (assoc op :type :info :error {:kind "adapter-io" :message (.getMessage e)})))))
  (teardown! [this test])
  (close! [this test] (adapter-close conn)))

(defrecord AdapterNemesis [port timeout-ms fs conn]
  nemesis/Nemesis
  (setup! [this test]
    (assoc this :conn (adapter-connect port timeout-ms)))
  (invoke! [this test op]
    ; :value carries the transport kind of a :transport op.
    (let [reply (request! conn (cond-> {:op (name (:f op))}
                                 (some? (:value op)) (assoc :value (:value op))))]
      (assoc op :type :info
                :value (:value reply)
                :error (:error reply)
                :generation (:generation reply))))
  (teardown! [this test] (adapter-close conn))
  nemesis/Reflection
  (fs [this] fs))

(defn nemesis-op [f] {:type :info :f f})

; The closing client phase's length: a live-lane generation gets two 3 s
; client windows (the live cycle's sleeps) before its checkpoint-crash.
(def closing-seconds 6)

; Append values of the shaped families: one run-wide counter, so a value is
; unique per key across every phase and generation.
(def shaped-values (AtomicLong.))

(defn shaped-txns
  "A shaped family's transactions (scripts/jepsen_checkpoint.py SHAPED):
  a weighted choice of one shape, whose micro-ops are [f role]; the key is
  base + role and every append of the transaction carries the same fresh
  value. Every generation has fresh row objects, so keys need no phase offset."
  [{:keys [base shapes]}]
  (let [weighted (vec (mapcat (fn [{:keys [weight mops]}] (repeat weight mops)) shapes))]
    (fn []
      (let [v (.incrementAndGet ^AtomicLong shaped-values)]
        {:type  :invoke
         :f     :txn
         :value (mapv (fn [[f role]] [(keyword f) (+ base role) (when (= f "append") v)])
                      (jrand/nth weighted))}))))

(defn family-txns
  "Elle's list-append transactions for one row family. Values are unique per
  key only within one la/gen instance, and each instance starts its keys and
  values afresh, so each family and client phase gets a disjoint key range
  (base + phase offset): (key, value) then identifies one append across every
  generation of the run. The adapter maps a key to its family's MOO rows.
  A family with shapes uses shaped-txns instead."
  [{:keys [base key-count shapes] :as family} phase-offset]
  (if (seq shapes)
    (shaped-txns family)
    (->> (la/gen {:key-count key-count :min-txn-length 1 :max-txn-length 4
                  :max-writes-per-key 64})
         (gen/map (fn [op]
                    (update op :value
                            (fn [txn] (mapv (fn [[f k v]] [f (+ base phase-offset k) v]) txn))))))))

(defn read-replay
  "--replay: the recorded client invocations (scripts/jepsen_checkpoint.py
  writes them as replay.jsonl), grouped by the :phase each was generated in,
  in recorded order. Only the transaction is kept: process and time are
  assigned afresh, so the OS schedule may differ from the recording."
  [path]
  (with-open [r (io/reader path)]
    (->> (line-seq r)
         (remove #(re-matches #"\s*" %))
         (map #(json/read-str % :key-fn keyword))
         (map (fn [op] {:type :invoke :f :txn :phase (:phase op) :value (mapv decode-mop (:value op))}))
         (group-by :phase))))

(defn txns
  "A uniform random mixture of the selected families' transactions, or with
  --replay the phase's recorded transactions in order. Every op carries its
  phase (the key phase offset), which the history records for a later replay."
  [{:keys [rate families replay]} phase-offset]
  (->> (if replay
         (get replay phase-offset [])
         (gen/mix (mapv #(family-txns % phase-offset) families)))
       (gen/map #(assoc % :phase phase-offset))
       (gen/stagger (/ 1 rate))))

(defn stage-generator
  "The elle build's deterministic stage schedule: one client phase per step,
  each with one :stage-crash, which the adapter runs at the stop point armed
  for that generation (scripts/jepsen_checkpoint.py STAGE_SCHEDULE); then a
  client phase on the last recovered generation and the final checkpoint.
  Each phase's keys are disjoint (3000000 + 100000 per step)."
  [{:keys [stages stage-seconds] :as options}]
  (apply gen/phases
         (concat
           (for [step (range stages)]
             (gen/time-limit stage-seconds
                             (gen/clients (txns options (+ 3000000 (* step 100000)))
                                          [(gen/sleep 3) (nemesis-op :stage-crash)])))
           [(gen/time-limit stage-seconds (gen/clients (txns options (+ 3000000 (* stages 100000)))))
            (gen/nemesis (nemesis-op :resume))
            (gen/nemesis (nemesis-op :checkpoint))])))

(defn generator
  "Phase 1: clients with pause/resume. Phase 2: an explicit checkpoint. Phase
  3: clients with pause/resume and crash/restart. Live phase (unless
  phase-live is 0): clients continue through SIGALRM live checkpoints and
  checkpoint-crashes, which stop and kill Barn at a random moment of a
  checkpoint and recover from its actual artifacts. Transport phase (unless
  phase-transport is 0): clients continue while the nemesis cycles the
  selected transport kinds, each followed by the adapter's bounded recovery
  check. Closing phase: clients only, on the last generation. Phase 4: a
  final checkpoint. gen/phases waits for every outstanding operation before
  the next phase.

  A time limit can expire during the crash or checkpoint-crash that ends a
  cycle; gen/phases then waits for the restart, and without the closing
  phase the restarted generation would reach the final checkpoint with no
  client operation (scripts/jepsen_checkpoint.py obligations)."
  [{:keys [phase1 phase3 phase-live phase-transport transport-kinds stages] :as options}]
  (if (pos? stages)
    (stage-generator options)
  (let [pausing   [(gen/sleep 5) (nemesis-op :pause) (gen/sleep 4) (nemesis-op :resume)]
        crashing  (into pausing [(gen/sleep 5) (nemesis-op :crash)])
        live      [(gen/sleep 3) (nemesis-op :live-checkpoint) (gen/sleep 3) (nemesis-op :checkpoint-crash)]
        transport (vec (mapcat (fn [kind] [(gen/sleep 2) {:type :info :f :transport :value kind}])
                               transport-kinds))]
    (apply gen/phases
           (concat
             [(gen/time-limit phase1 (gen/clients (txns options 0) (gen/cycle pausing)))
              (gen/nemesis (nemesis-op :resume))
              (gen/nemesis (nemesis-op :checkpoint))
              (gen/time-limit phase3 (gen/clients (txns options 1000000) (gen/cycle crashing)))
              (gen/nemesis (nemesis-op :resume))]
             (when (pos? phase-live)
               [(gen/time-limit phase-live (gen/clients (txns options 2000000) (gen/cycle live)))])
             (when (pos? phase-transport)
               [(gen/time-limit phase-transport
                                (gen/clients (txns options 5000000) (gen/cycle transport)))])
             [(gen/time-limit closing-seconds (gen/clients (txns options 4000000)))
              (gen/nemesis (nemesis-op :checkpoint))])))))

(defn barn-test [{:keys [adapter-port concurrency client-timeout-ms] :as options}]
  (assoc tests/noop-test
         :name        "barn-checkpoint"
         :nodes       ["n1"]
         :ssh         {:dummy? true}
         :concurrency concurrency
         :client      (map->Client {:port adapter-port :timeout-ms client-timeout-ms
                                    :ids (AtomicLong.)})
         :nemesis     (nemesis/compose
                        [(map->AdapterNemesis {:port adapter-port :timeout-ms 600000
                                               :fs #{:pause :resume}})
                         (map->AdapterNemesis {:port adapter-port :timeout-ms 600000
                                               :fs #{:crash :checkpoint :live-checkpoint
                                                     :checkpoint-crash :stage-crash :transport}})])
         :generator   (generator options)
         ; Verdicts come from the pinned Elle CLI and the checkpoint baselines.
         :checker     (checker/unbridled-optimism)))

(defn jsonable
  "Plain JSON data for one history op; anything else is kept as its string."
  [x]
  (cond (keyword? x)    (name x)
        (map? x)        (into {} (map (fn [[k v]] [(if (keyword? k) (name k) (str k)) (jsonable v)])) x)
        (sequential? x) (mapv jsonable x)
        (set? x)        (mapv jsonable (sort-by str x))
        (or (nil? x) (string? x) (number? x) (boolean? x)) x
        :else           (str x)))

(defn write-history! [path history]
  (with-open [w (io/writer path)]
    (doseq [op history]
      (.write w (str (json/write-str (jsonable (into {} op))) "\n")))))

(def cli-options
  [[nil "--adapter-port PORT" :parse-fn parse-long]
   [nil "--history PATH"]
   [nil "--phase1 SECONDS" :default 60 :parse-fn parse-long]
   [nil "--phase3 SECONDS" :default 90 :parse-fn parse-long]
   [nil "--phase-live SECONDS" :default 60 :parse-fn parse-long]
   [nil "--phase-transport SECONDS" :default 0 :parse-fn parse-long]
   ; The transport kinds to cycle, ["pre-send", ...], from
   ; scripts/jepsen_checkpoint.py, which owns TRANSPORT_KINDS.
   [nil "--transport-kinds JSON" :default [] :parse-fn json/read-str]
   ; With --stages N > 0, the stage schedule replaces the phases above.
   [nil "--stages N" :default 0 :parse-fn parse-long]
   [nil "--stage-seconds SECONDS" :default 10 :parse-fn parse-long]
   [nil "--rate OPS" :default 20 :parse-fn parse-long]
   [nil "--concurrency N" :default 8 :parse-fn parse-long]
   [nil "--client-timeout-ms MS" :default 60000 :parse-fn parse-long]
   [nil "--seed N" :default 1 :parse-fn parse-long]
   ; The selected row families, [{"name" ..., "base" ..., "key-count" ...}],
   ; from scripts/jepsen_checkpoint.py, which owns the family table.
   [nil "--families JSON" :parse-fn #(json/read-str % :key-fn keyword)]
   ; Recorded client invocations to send instead of generated ones, each
   ; phase its own (read-replay); the phases and nemesis schedule are kept.
   [nil "--replay PATH" :parse-fn read-replay]])

(defn -main [& args]
  (let [{:keys [options errors]} (cli/parse-opts args cli-options)]
    (when (or (seq errors) (nil? (:adapter-port options)) (nil? (:history options))
              (empty? (:families options)))
      (binding [*out* *err*]
        (println "usage: --adapter-port PORT --history PATH --families JSON" errors))
      (System/exit 2))
    ; Binds jepsen.random (stagger timing) only; Elle's transaction generator
    ; and real concurrency are not reproduced by the seed. --replay reproduces
    ; the transactions, not the interleaving.
    (let [test (jrand/with-seed (:seed options) (jepsen/run! (barn-test options)))]
      (write-history! (:history options) (:history test))
      (info "wrote" (count (:history test)) "history ops to" (:history options))
      (shutdown-agents)
      (System/exit 0))))
