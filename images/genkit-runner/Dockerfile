# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

ARG BASE_IMAGE=python:3.12-slim
FROM ${BASE_IMAGE}

RUN apt-get update && apt-get install -y --no-install-recommends \
        git \
        curl \
        ca-certificates \
        openssh-client \
        procps \
        bash \
        jq \
    && rm -rf /var/lib/apt/lists/*

LABEL org.opencontainers.image.source="https://github.com/ghchinoy/ax-images"

ARG TARGETARCH
COPY bin/linux_${TARGETARCH}/genkit-runner /usr/local/bin/ax-agent
RUN ln -f /usr/local/bin/ax-agent /usr/local/bin/ax-task-runner

WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/ax-task-runner"]
