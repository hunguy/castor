FROM debian:bookworm-slim
# buildx sets TARGETARCH per platform; the matching linux binary is staged into
# the build context by the release workflow (docker/<arch>/castor).
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends libgomp1 libstdc++6 && rm -rf /var/lib/apt/lists/*
COPY docker/${TARGETARCH}/castor /usr/local/bin/castor
RUN chmod +x /usr/local/bin/castor
ENTRYPOINT ["castor"]
