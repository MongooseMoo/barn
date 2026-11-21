(module
  (memory (export "memory") 1)
  (global $g (mut i32) (i32.const 0))
  (func (export "bump-global") (result i32)
    global.get $g i32.const 1 i32.add global.set $g
    global.get $g)
  (func (export "bump-memory") (result i32)
    i32.const 16
    i32.const 16 i32.load i32.const 1 i32.add
    i32.store
    i32.const 16 i32.load))
