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


def main() -> None:
    broker = os.environ.get("KAFKA_BROKER", "kafka:9092")
    topic = os.environ.get("KAFKA_TOPIC", "scripts")

    producer = create_producer(broker)

    scripts = [
        {
            "type": "script",
            "id": "hello",
            "source": "print('Hello from Python inside Docker!')\n",
        },
        {
            "type": "script",
            "id": "time",
            "source": "import datetime\nprint('Current time:', datetime.datetime.now(datetime.timezone.utc).isoformat())\n",
        },
    ]

    for script in scripts:
        producer.send(topic, key=script["id"], value=script)
        producer.flush()
        time.sleep(0.1)

    producer.send(topic, key="__control__", value={"type": "done"})
    producer.flush()
    producer.close()


if __name__ == "__main__":
    main()

