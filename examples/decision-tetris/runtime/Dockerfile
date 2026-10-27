# Community decision runtime on the same Red Hat AI CUDA base as the workshop.
FROM registry.redhat.io/rhaii/vllm-cuda-rhel9:3.5.0@sha256:5d4434598dc702b89c8d63b9fa6e2378130abf649c95f4211fb93e325cfd7698
USER 0
RUN python3 -m pip install --no-cache-dir --no-deps 'https://github.com/vllm-project/semantic-router/archive/ff293191da0232b4e252bee2edd6ed825cacd45d.tar.gz#subdirectory=src/model-runtime' && \
    python3 -m pip install --no-cache-dir 'backports.zstd==1.8.0'
USER 1001
ENTRYPOINT ["python3", "-m", "vllm_srun.cli"]
