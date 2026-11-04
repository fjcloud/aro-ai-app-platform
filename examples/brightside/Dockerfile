FROM registry.access.redhat.com/ubi9/nodejs-22:latest
WORKDIR /opt/app-root/src
COPY --chown=1001:0 package.json server.mjs engine.mjs providers.mjs agent.mjs bridge.mjs http-security.mjs index.html sources.json THIRD_PARTY_NOTICES.md ./
ENV NODE_ENV=production HOST=0.0.0.0 PORT=8080 STATE_FILE=/var/lib/brightside/state.json
USER 1001
EXPOSE 8080
ENTRYPOINT ["node"]
CMD ["server.mjs"]
