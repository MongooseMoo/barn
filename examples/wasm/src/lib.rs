// Standard WIT bindings implement the Canonical ABI. No Barn guest SDK is needed.
wit_bindgen::generate!({ path: "wit", world: "builtins" });

struct Builtins;
impl Guest for Builtins {
    fn add(a: i64, b: i64) -> i64 { a.wrapping_add(b) }
    fn greet(name: String) -> String { format!("Hello, {name}!") }
    fn echo(values: Vec<i64>) -> Vec<i64> { values }
    fn record_echo(value: Greeting) -> Greeting { value }
    fn color_echo(value: Color) -> Color { value }
    fn choice_echo(value: Choice) -> Choice { value }
    fn option_echo(value: Option<i64>) -> Option<i64> { value }
    fn checked(value: i64) -> Result<i64, String> {
        if value < 0 { Err("negative".into()) } else { Ok(value) }
    }
    fn counter() -> u32 {
        use std::sync::atomic::{AtomicU32, Ordering};
        static COUNT: AtomicU32 = AtomicU32::new(0);
        COUNT.fetch_add(1, Ordering::Relaxed) + 1
    }
    fn spin() -> i64 {
        loop { std::hint::spin_loop(); }
    }
    fn trap() -> i64 { panic!("guest trap") }
}
export!(Builtins);
