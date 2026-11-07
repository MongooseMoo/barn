# Native command-environment fixture

Produced by the managed canonical WSL Toast server from public Test.db using
tests/mongoose-conformance/server/native_command_environment.yaml. Root retained
the checkpoint before its suspended start verb continued, and reported canonical
admission plus the complete native restart case passed in 24.15 seconds.

SHA256: b395edf39a8f79b2bb5670d78573b157c1064f90f41a3538bd25301cbff67de1

The root driver sets six command variables to nonempty values and forks a call
to start. The start verb never refers to those variable names: it calls an
environment helper, suspends, calls the helper again and increments done. The
saved before value is {"native command", #1, "direct", "with", #1, "indirect"};
after is zero and done is zero. Barn's post-import continuation must make after
equal before and done equal one, proving inheritance through a resumed frame
whose compiled program has no named command-variable slots.

Only test-owned public Test.db data is included; no production world or player
data. Tests change only the absolute wake deadline to make the saved task due.
