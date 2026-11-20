(defproject barn-checkpoint "0.1.0"
  :description "Jepsen client and nemesis for Barn's checkpoint durability campaign.
  scripts/jepsen_checkpoint.py owns the Barn processes and the MOO transport;
  this project owns scheduling, concurrency and the recorded history."
  :dependencies [[org.clojure/clojure "1.12.6"]
                 [jepsen "0.3.15"]
                 [org.clojure/data.json "2.5.0"]]
  :main barn-checkpoint.core
  :aot [barn-checkpoint.core]
  ; Upstream Jepsen uses -Xmx32g; this one-node workload is bounded to 2 GiB.
  ; scripts/jepsen_checkpoint.py passes the same bound to the uberjar's JVM.
  :jvm-opts ["-Xmx2g" "-Djava.awt.headless=true" "-server"])
