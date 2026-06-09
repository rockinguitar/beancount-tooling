FROM python:3.14.5-slim@sha256:c845af9399020c7e562969a13689e929074a10fd057acd1b1fad06a2fb068e97 AS builder

ENV DEBIAN_FRONTEND=noninteractive

# Beancount 3.x build deps on linux/arm64: compiler + bison (and flex is often needed too)
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    pkg-config \
    bison \
    flex \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir --root-user-action ignore --prefix="/install" fava[excel]

FROM python:3.14.5-slim@sha256:c845af9399020c7e562969a13689e929074a10fd057acd1b1fad06a2fb068e97
COPY --from=builder /install /usr/local

ENV FAVA_HOST=0.0.0.0
EXPOSE 5000
CMD ["fava"]