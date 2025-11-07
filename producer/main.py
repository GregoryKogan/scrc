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
                raise RuntimeError("failed to connect to Kafka broker within 60 seconds") from exc

            time.sleep(backoff)
            backoff = min(backoff * 1.5, max_backoff)


def stream_scripts(interval_s: float = 1.0):
    counter = 1
    while True:
        yield {
            "type": "script",
            "id": f"script-{counter}",
            "source": (
                "import datetime\n"
                "print('Streaming script {counter} at', datetime.datetime.now(datetime.timezone.utc).isoformat())\n"
            ).format(counter=counter),
        }
        counter += 1
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

