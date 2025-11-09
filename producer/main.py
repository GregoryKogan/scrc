import datetime
import json
import os
import sys
import time

import six

sys.modules.setdefault("kafka.vendor.six", six)
sys.modules.setdefault("kafka.vendor.six.moves", six.moves)

from kafka import KafkaProducer
from kafka.errors import NoBrokersAvailable


def create_producer(broker: str) -> KafkaProducer:
    backoff = 1.0
    max_backoff = 10.0
    deadline = time.time() + 60

    while True:
        try:
            return KafkaProducer(
                bootstrap_servers=broker,
                key_serializer=lambda v: v.encode("utf-8") if v else None,
                value_serializer=lambda v: json.dumps(v).encode("utf-8"),
            )
        except NoBrokersAvailable as exc:
            if time.time() >= deadline:
                raise RuntimeError(
                    "failed to connect to Kafka broker within 60 seconds"
                ) from exc

            time.sleep(backoff)
            backoff = min(backoff * 1.5, max_backoff)


def build_scenarios() -> list[dict]:
    sum_script = "\n".join(
        [
            "import sys",
            "",
            "def main() -> None:",
            "    data = sys.stdin.read().strip()",
            "    if not data:",
            "        print('0')",
            "        return",
            "    total = sum(int(part) for part in data.split())",
            "    print(total)",
            "",
            "if __name__ == '__main__':",
            "    main()",
        ]
    )

    time_limit_script = "\n".join(
        [
            "import time",
            "print('About to nap for a bit...')",
            "time.sleep(2.5)",
            "print('Woke up!')",
        ]
    )

    memory_limit_script = "\n".join(
        [
            "print('Allocating memory...')",
            "blocks = []",
            "for _ in range(128):",
            "    blocks.append(bytearray(1024 * 1024))  # 1 MiB per block",
            "print('Allocated', len(blocks), 'MiB successfully')",
        ]
    )

    go_sum_script = "\n".join(
        [
            "package main",
            "",
            "import (",
            '    "bufio"',
            '    "fmt"',
            '    "os"',
            ")",
            "",
            "func main() {",
            "    reader := bufio.NewReader(os.Stdin)",
            "    var total int64",
            "    for {",
            "        var value int64",
            "        _, err := fmt.Fscan(reader, &value)",
            "        if err != nil {",
            "            break",
            "        }",
            "        total += value",
            "    }",
            "    fmt.Println(total)",
            "}",
        ]
    )

    c_sum_script = "\n".join(
        [
            "#include <stdio.h>",
            "",
            "int main(void) {",
            "    long long value;",
            "    long long total = 0;",
            '    while (scanf("%lld", &value) == 1) {',
            "        total += value;",
            "    }",
            '    printf("%lld\\n", total);',
            "    return 0;",
            "}",
        ]
    )

    cpp_sum_script = "\n".join(
        [
            "#include <iostream>",
            "",
            "int main() {",
            "    long long value;",
            "    long long total = 0;",
            "    while (std::cin >> value) {",
            "        total += value;",
            "    }",
            '    std::cout << total << "\\n";',
            "    return 0;",
            "}",
        ]
    )

    java_sum_script = "\n".join(
        [
            "import java.util.Scanner;",
            "",
            "public class Main {",
            "    public static void main(String[] args) {",
            "        Scanner scanner = new Scanner(System.in);",
            "        long total = 0;",
            "        while (scanner.hasNextLong()) {",
            "            total += scanner.nextLong();",
            "        }",
            "        System.out.println(total);",
            "    }",
            "}",
        ]
    )

    return [
        {
            "base_id": "script-ok",
            "language": "python",
            "source": sum_script,
            "limits": {
                "time_limit_ms": 2_000,
                "memory_limit_bytes": 256 * 1024 * 1024,
            },
            "tests": [
                {
                    "number": 1,
                    "input": "1 2 3\n",
                    "expected_output": "6\n",
                },
                {
                    "number": 2,
                    "input": "10 -5 7\n",
                    "expected_output": "12\n",
                },
            ],
        },
        {
            "base_id": "script-tl",
            "language": "python",
            "source": time_limit_script,
            "limits": {
                "time_limit_ms": 1_000,
                "memory_limit_bytes": 256 * 1024 * 1024,
            },
            "tests": [
                {
                    "number": 1,
                    "input": "",
                    "expected_output": "About to nap for a bit...\n",
                }
            ],
        },
        {
            "base_id": "script-ml",
            "language": "python",
            "source": memory_limit_script,
            "limits": {
                "time_limit_ms": 5_000,
                "memory_limit_bytes": 32 * 1024 * 1024,
            },
            "tests": [
                {
                    "number": 1,
                    "input": "",
                    "expected_output": "Allocating memory...\n",
                }
            ],
        },
        {
            "base_id": "script-go-ok",
            "language": "go",
            "source": go_sum_script,
            "limits": {
                "time_limit_ms": 2_000,
                "memory_limit_bytes": 256 * 1024 * 1024,
            },
            "tests": [
                {
                    "number": 1,
                    "input": "1 2 3\n",
                    "expected_output": "6\n",
                },
                {
                    "number": 2,
                    "input": "10 -5 7\n",
                    "expected_output": "12\n",
                },
            ],
        },
        {
            "base_id": "script-c-ok",
            "language": "c",
            "source": c_sum_script,
            "limits": {
                "time_limit_ms": 2_000,
                "memory_limit_bytes": 256 * 1024 * 1024,
            },
            "tests": [
                {
                    "number": 1,
                    "input": "1 2 3\n",
                    "expected_output": "6\n",
                },
                {
                    "number": 2,
                    "input": "10 -5 7\n",
                    "expected_output": "12\n",
                },
            ],
        },
        {
            "base_id": "script-cpp-ok",
            "language": "cpp",
            "source": cpp_sum_script,
            "limits": {
                "time_limit_ms": 2_000,
                "memory_limit_bytes": 256 * 1024 * 1024,
            },
            "tests": [
                {
                    "number": 1,
                    "input": "1 2 3\n",
                    "expected_output": "6\n",
                },
                {
                    "number": 2,
                    "input": "10 -5 7\n",
                    "expected_output": "12\n",
                },
            ],
        },
        {
            "base_id": "script-java-ok",
            "language": "java",
            "source": java_sum_script,
            "limits": {
                "time_limit_ms": 2_000,
                "memory_limit_bytes": 256 * 1024 * 1024,
            },
            "tests": [
                {
                    "number": 1,
                    "input": "1 2 3\n",
                    "expected_output": "6\n",
                },
                {
                    "number": 2,
                    "input": "10 -5 7\n",
                    "expected_output": "12\n",
                },
            ],
        },
    ]


def stream_scripts(interval_s: float = 1.0):
    scenarios = build_scenarios()
    iteration = 1
    while True:
        for scenario in scenarios:
            yield {
                "type": "script",
                "id": f"{scenario['base_id']}-{iteration}",
                "language": scenario["language"],
                "source": scenario["source"],
                "limits": scenario["limits"],
                "tests": scenario["tests"],
            }
        iteration += 1
        time.sleep(interval_s)


def main() -> None:
    broker = os.environ.get("KAFKA_BROKER", "kafka:9092")
    topic = os.environ.get("KAFKA_TOPIC", "scripts")
    raw_interval = os.environ.get("SCRIPT_INTERVAL_SECONDS", "1.0")
    try:
        interval = float(raw_interval)
        if interval < 0:
            raise ValueError("negative interval")
    except ValueError:
        print(
            f"invalid SCRIPT_INTERVAL_SECONDS value {raw_interval!r}; defaulting to 1.0",
            file=sys.stderr,
        )
        interval = 1.0

    producer = create_producer(broker)

    try:
        for script in stream_scripts(interval):
            future = producer.send(topic, key=script["id"], value=script)
            future.get(timeout=30)
    except KeyboardInterrupt:
        print("stopping script stream", file=sys.stderr)
    finally:
        producer.flush()
        producer.close()


if __name__ == "__main__":
    main()
