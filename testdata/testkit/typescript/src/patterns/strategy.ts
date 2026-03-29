export interface Strategy<T> {
  execute(input: T): T;
}

export class UpperCaseStrategy implements Strategy<string> {
  execute(input: string): string {
    return input.toUpperCase();
  }
}

export class ReverseStrategy implements Strategy<string> {
  execute(input: string): string {
    return input.split("").reverse().join("");
  }
}

export class Processor<T> {
  constructor(private strategy: Strategy<T>) {}

  process(input: T): T {
    return this.strategy.execute(input);
  }

  setStrategy(strategy: Strategy<T>): void {
    this.strategy = strategy;
  }
}
