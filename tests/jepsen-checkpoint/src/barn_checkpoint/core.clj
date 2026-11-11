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
    (let [reply (request! conn {:op (name (:f op))})]
      (assoc op :type :info
                :value (:value reply)
                :error (:error reply)
                :generation (:generation reply))))
  (teardown! [this test] (adapter-close conn))
  nemesis/Reflection
  (fs [this] fs))

(defn nemesis-op [f] {:type :info :f f})

(defn txns
  "Elle's list-append transactions. Values are unique per key only within one
  la/gen instance, and each instance starts its keys and values afresh, so
  each client phase gets a disjoint key range: (key, value) then identifies
  one append across every generation of the run."
  [rate key-offset]
  (->> (la/gen {:key-count 5 :min-txn-length 1 :max-txn-length 4
                :max-writes-per-key 64})
       (gen/map (fn [op]
                  (update op :value
                          (fn [txn] (mapv (fn [[f k v]] [f (+ key-offset k) v]) txn)))))
       (gen/stagger (/ 1 rate))))

(defn generator
  "Phase 1: clients with pause/resume. Phase 2: an explicit checkpoint. Phase
  3: clients with pause/resume and crash/restart. Phase 4: a final checkpoint.
  gen/phases waits for every outstanding operation before the next phase."
  [{:keys [phase1 phase3 rate]}]
  (let [pausing  [(gen/sleep 5) (nemesis-op :pause) (gen/sleep 4) (nemesis-op :resume)]
        crashing (into pausing [(gen/sleep 5) (nemesis-op :crash)])]
    (gen/phases
      (gen/time-limit phase1 (gen/clients (txns rate 0) (gen/cycle pausing)))
      (gen/nemesis (nemesis-op :resume))
      (gen/nemesis (nemesis-op :checkpoint))
      (gen/time-limit phase3 (gen/clients (txns rate 1000000) (gen/cycle crashing)))
      (gen/nemesis (nemesis-op :resume))
      (gen/nemesis (nemesis-op :checkpoint)))))

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
                                               :fs #{:crash :checkpoint}})])
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
   [nil "--rate OPS" :default 20 :parse-fn parse-long]
   [nil "--concurrency N" :default 8 :parse-fn parse-long]
   [nil "--client-timeout-ms MS" :default 60000 :parse-fn parse-long]
   [nil "--seed N" :default 1 :parse-fn parse-long]])

(defn -main [& args]
  (let [{:keys [options errors]} (cli/parse-opts args cli-options)]
    (when (or (seq errors) (nil? (:adapter-port options)) (nil? (:history options)))
      (binding [*out* *err*]
        (println "usage: --adapter-port PORT --history PATH" errors))
      (System/exit 2))
    ; Binds jepsen.random (stagger timing) only; Elle's transaction generator
    ; and real concurrency are not reproduced by the seed.
    (let [test (jrand/with-seed (:seed options) (jepsen/run! (barn-test options)))]
      (write-history! (:history options) (:history test))
      (info "wrote" (count (:history test)) "history ops to" (:history options))
      (shutdown-agents)
      (System/exit 0))))
