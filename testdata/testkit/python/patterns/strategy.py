"""Strategy pattern via duck typing."""
from typing import Protocol


class SortStrategy(Protocol):
    def sort(self, data: list) -> list: ...


class QuickSort:
    def sort(self, data: list) -> list:
        if len(data) <= 1:
            return data
        pivot = data[0]
        left = [x for x in data[1:] if x <= pivot]
        right = [x for x in data[1:] if x > pivot]
        return self.sort(left) + [pivot] + self.sort(right)


class BubbleSort:
    def sort(self, data: list) -> list:
        result = list(data)
        for i in range(len(result)):
            for j in range(len(result) - 1 - i):
                if result[j] > result[j + 1]:
                    result[j], result[j + 1] = result[j + 1], result[j]
        return result


class Sorter:
    def __init__(self, strategy: SortStrategy):
        self._strategy = strategy

    def execute(self, data: list) -> list:
        return self._strategy.sort(data)
