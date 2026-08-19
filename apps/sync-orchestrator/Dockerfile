FROM python:3.12-slim
RUN pip install --no-cache-dir tzdata
RUN useradd -u 1000 -m appuser
COPY server.py /server.py
USER appuser
ENTRYPOINT ["python3", "/server.py"]
