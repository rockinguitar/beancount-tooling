FROM python:slim AS builder

ENV DEBIAN_FRONTEND=noninteractive

# Beancount 3.x build deps on linux/arm64: compiler + bison (and flex is often needed too)
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    pkg-config \
    bison \
    flex \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir --root-user-action ignore --prefix="/install" fava

FROM python:slim
COPY --from=builder /install /usr/local

ENV FAVA_HOST=0.0.0.0
EXPOSE 5000
CMD ["fava"]