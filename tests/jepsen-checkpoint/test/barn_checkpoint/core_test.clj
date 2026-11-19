(ns barn-checkpoint.core-test
  "Runs the campaign generator through Jepsen's interpreter against an
  in-process client and nemesis, so the schedule's timing is the real one."
  (:require [clojure.test :refer [deftest is]]
            [barn-checkpoint.core :as core]
            [jepsen [client :as client]
                    [core :as jepsen]
                    [nemesis :as nemesis]
                    [store :as store]]))

(defrecord RecordingClient [invokes]
  client/Client
  (open! [this test node] this)
  (setup! [this test] this)
  (invoke! [this test op]
    (swap! invokes conj (System/nanoTime))
    (assoc op :type :ok :value (mapv (fn [[f k v]] [f k (if (= f :r) [] v)]) (:value op))))
  (teardown! [this test])
  (close! [this test]))

(defrecord RecoveringNemesis [invokes hold bound results]
  ; Models the adapter's transport op (scripts/jepsen_checkpoint.py
  ; Campaign.transport and recover): hold the fault, heal, then wait up to
  ; bound for a client operation invoked after the heal.
  nemesis/Nemesis
  (setup! [this test] this)
  (invoke! [this test op]
    (if (= :transport (:f op))
      (do (Thread/sleep (long (* 1000 hold)))
          (let [heal     (System/nanoTime)
                deadline (+ heal (long (* 1e9 bound)))
                ok?      (loop []
                           (cond (some #(>= % heal) @invokes) true
                                 (>= (System/nanoTime) deadline) false
                                 :else (do (Thread/sleep 10) (recur))))]
            (swap! results conj ok?)
            (assoc op :type :info :value {:recovered ok?})))
      (assoc op :type :info)))
  (teardown! [this test])
  nemesis/Reflection
  (fs [this] #{:transport :pause :resume :crash :checkpoint}))

(deftest transport-fault-at-the-phase-limit-gets-client-operations-after-its-heal
  ; The cycle sleeps 2 s, then begins one fault; with a 2.5 s phase that fault
  ; begins before the limit and heals after it (hold 0.8 s).
  (let [hold 0.8, bound 1.0
        invokes (atom [])
        results (atom [])
        dir     (str (java.nio.file.Files/createTempDirectory
                       "barn-checkpoint-test" (make-array java.nio.file.attribute.FileAttribute 0)))
        test    (assoc (core/barn-test {:adapter-port 0 :concurrency 2 :client-timeout-ms 1000
                                        :phase1 0 :phase3 0 :phase-live 0 :phase-transport 2.5
                                        :transport-tail (+ hold bound) :transport-kinds ["slow-reader"]
                                        :rate 50 :stages 0 :stage-seconds 0
                                        :families [{:name "smoke" :base 0 :key-count 3}]})
                       :client (->RecordingClient invokes)
                       :nemesis (->RecoveringNemesis invokes hold bound results))]
    (with-redefs [store/base-dir dir]
      (jepsen/run! test))
    (is (= [true] @results))))
