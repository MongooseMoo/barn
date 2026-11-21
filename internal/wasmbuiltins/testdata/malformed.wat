(component
  (core module $m
    (memory (export "memory") 1)
    (func (export "bad") (result i32)
      i32.const 0 i32.const -16 i32.store
      i32.const 4 i32.const 1024 i32.store
      i32.const 0)
    (func (export "wide") (result i32)
      i32.const 0 i32.const 0 i32.store
      i32.const 4 i32.const 1073741824 i32.store
      i32.const 0)
    (func (export "text") (result i32)
      i32.const 0 i32.const -1 i32.store
      i32.const 4 i32.const 8 i32.store
      i32.const 0)
    (func (export "healthy") (result i64) i64.const 42))
  (core instance $i (instantiate $m))
  (alias core export $i "memory" (core memory $mem))
  (alias core export $i "bad" (core func $bad))
  (alias core export $i "wide" (core func $wide))
  (alias core export $i "text" (core func $text))
  (alias core export $i "healthy" (core func $healthy))
  (type $bytes (list u8))
  (type $words (list u64))
  (type $f (func (result $bytes)))
  (type $g (func (result $words)))
  (type $h (func (result string)))
  (type $j (func (result s64)))
  (func (export "bad") (type $f) (canon lift (core func $bad) (memory $mem)))
  (func (export "wide") (type $g) (canon lift (core func $wide) (memory $mem)))
  (func (export "text") (type $h) (canon lift (core func $text) (memory $mem)))
  (func (export "healthy") (type $j) (canon lift (core func $healthy))))
