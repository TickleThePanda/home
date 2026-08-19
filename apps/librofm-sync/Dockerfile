FROM python:3.12-slim

RUN pip install --no-cache-dir librofm==0.2.0

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
