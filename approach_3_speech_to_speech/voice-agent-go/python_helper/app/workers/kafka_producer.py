import json
from aiokafka import AIOKafkaProducer
from app.config import settings

class KafkaProducerWrapper:
    def __init__(self):
        self.producer = None

    async def start(self):
        self.producer = AIOKafkaProducer(
            bootstrap_servers=settings.KAFKA_BOOTSTRAP_SERVERS,
            value_serializer=lambda v: json.dumps(v).encode('utf-8'),
            key_serializer=lambda k: k.encode('utf-8') if k else None
        )
        await self.producer.start()

    async def stop(self):
        if self.producer:
            await self.producer.stop()

    async def publish(self, topic: str, value: dict, key: str = None):
        if not self.producer:
            raise RuntimeError("Kafka producer not started")
        await self.producer.send_and_wait(topic, value=value, key=key)

kafka_producer_instance = KafkaProducerWrapper()

async def get_kafka_producer():
    return kafka_producer_instance
