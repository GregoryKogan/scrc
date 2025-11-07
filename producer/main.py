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
    stomp_cpu_script = "\n".join(
        [
            "import datetime",
            "print('Quick script executed at', datetime.datetime.now(datetime.timezone.utc).isoformat())",
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

    return [
        {
            "base_id": "script-ok",
            "source": stomp_cpu_script,
            "limits": {
                "time_limit_ms": 2_000,
                "memory_limit_bytes": 256 * 1024 * 1024,
            },
        },
        {
            "base_id": "script-tl",
            "source": time_limit_script,
            "limits": {
                "time_limit_ms": 1_000,
                "memory_limit_bytes": 256 * 1024 * 1024,
            },
        },
        {
            "base_id": "script-ml",
            "source": memory_limit_script,
            "limits": {
                "time_limit_ms": 5_000,
                "memory_limit_bytes": 32 * 1024 * 1024,
            },
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
                "source": scenario["source"],
                "limits": scenario["limits"],
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
