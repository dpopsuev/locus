pub trait Strategy {
    fn execute(&self, input: &str) -> String;
}

pub struct UpperCase;

impl Strategy for UpperCase {
    fn execute(&self, input: &str) -> String {
        input.to_uppercase()
    }
}

pub struct Reverse;

impl Strategy for Reverse {
    fn execute(&self, input: &str) -> String {
        input.chars().rev().collect()
    }
}

pub struct Processor {
    strategy: Box<dyn Strategy>,
}

impl Processor {
    pub fn new(strategy: Box<dyn Strategy>) -> Self {
        Self { strategy }
    }

    pub fn process(&self, input: &str) -> String {
        self.strategy.execute(input)
    }
}
