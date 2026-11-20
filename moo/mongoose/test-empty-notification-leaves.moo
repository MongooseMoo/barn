caller_perms().wizard || raise(E_PERM);
R = $test_runner;
container = $recycler:_create($container, caller_perms());
R:push_cleanup(["kind" -> "recycle", "obj" -> container]);
observer = $recycler:_create($root_class, caller_perms());
R:push_cleanup(["kind" -> "recycle", "obj" -> observer]);
add_property(observer, "notification_done", 0, {caller_perms(), "rw"});
for i in [1..8]
  leaf = $recycler:_create($root_class, caller_perms());
  R:push_cleanup(["kind" -> "recycle", "obj" -> leaf]);
  move(leaf, container);
endfor
for notification in ({"loc_enter", "loc_exit"})
  observer.notification_done = 0;
  fork (0)
    observer.notification_done = 1;
  endfork
  container:(notification)(observer, container);
  yielded = observer.notification_done;
  while (!observer.notification_done)
    suspend(0);
  endwhile
  R:assert_equal(yielded, 0, "Empty root leaves must not cause " + notification + " to suspend");
  R:assert_equal(length(container.contents), 8, "Traversal retains every leaf");
endfor
return 1;
