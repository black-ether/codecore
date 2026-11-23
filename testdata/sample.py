"""Demo module for codecore's Python rule pack."""

import os
import sys as system
from pathlib import Path
from collections import defaultdict as dd

VERSION = "1.0"
PORT: int = 8080
_timeout = 30
f = lambda x: x

TRAP_STRING = "def fake(): pass"
# def commented(): pass


def helper(a, b=1, *args, **kwargs):
    """Do the thing."""
    local = a + b
    return local


async def fetch(url):
    return await _get(url)


@decorator
def decorated():
    pass


class Animal:
    species = "unknown"
    _count = 0

    def __init__(self, name):
        self.name = name

    def speak(self):
        return "..."

    @classmethod
    def create(cls, name):
        return cls(name)

    @property
    def label(self):
        return self.name


class Dog(Animal):
    def speak(self):
        return "woof"


def _private():
    pass


if __name__ == "__main__":
    helper(1)
