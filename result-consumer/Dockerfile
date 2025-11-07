FROM python:3.12-alpine

WORKDIR /app

RUN pip install --no-cache-dir kafka-python==2.0.2 six==1.16.0

COPY main.py ./

ENV KAFKA_BROKER=kafka:9092
ENV KAFKA_RESULTS_TOPIC=script-results
ENV KAFKA_CONSUMER_GROUP=scrc-results-consumer

CMD ["python", "main.py"]

