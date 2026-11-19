(ns barn-checkpoint.core-test
  "Generator schedule tests on jepsen.generator.test's simulated clock: no
  adapter, no Barn process."
  (:require [barn-checkpoint.core :as core]
            [clojure.test :refer [deftest is]]
            [jepsen.generator.test :as gen.test]))

(defn nanos [seconds] (long (* seconds 1e9)))

; One transport op: the fault, its heal and the adapter's recovery check.
(def fault-seconds 5)

; The recovery check's client clause (scripts/jepsen_checkpoint.py recover)
; needs a client transaction sent after the heal, so the last second of a
; transport op must still see client invocations.
(def recovery-seconds 1)

(def transport-seconds 10)

(defn complete
  "Sleeps last their value, a transport op fault-seconds, anything else 1 ms."
  [ctx op]
  (cond (= :sleep (:type op))  (update op :time + (nanos (:value op)))
        (= :transport (:f op)) (update op :time + (nanos fault-seconds))
        (= :nemesis (:process op)) (update op :time + (nanos 0.001))
        :else (-> op (assoc :type :ok) (update :time + (nanos 0.001)))))

(defn transport-client?
  "An invocation of the transport phase's clients (keys 5000000 + n)."
  [op]
  (and (= :invoke (:type op))
       (<= 5000000 (second (first (:value op))) 5999999)))

(deftest transport-fault-in-flight-at-the-phase-limit-keeps-its-clients
  ; Cycle: sleep 0-2 s, fault 2-7 s, sleep 7-9 s, fault 9-14 s. The second
  ; fault is in flight when the 10 s phase limit falls.
  (let [history (gen.test/simulate
                  (gen.test/n+nemesis-context 4)
                  (core/generator {:phase1 0 :phase3 0 :phase-live 0 :stages 0
                                   :phase-transport transport-seconds
                                   :transport-kinds ["pause"]
                                   :rate 20
                                   :families [{:name "smoke" :base 0 :key-count 5}]})
                  complete)
        client-times (map :time (filter transport-client? history))
        faults       (partition 2 (filter #(= :transport (:f %)) history))
        limit        (+ (first client-times) (nanos transport-seconds))]
    (is (= 2 (count faults)))
    (is (< (:time (first (last faults))) limit (:time (second (last faults))))
        "the last fault starts before the phase limit and finishes after it")
    (doseq [[invoked completed] faults]
      (is (some #(< (- (:time completed) (nanos recovery-seconds)) % (:time completed))
                client-times)
          (str "no client transaction was invoked in the last " recovery-seconds
               " s of the transport op invoked at " (/ (:time invoked) 1e9) " s")))))
