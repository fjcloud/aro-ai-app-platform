FROM registry.access.redhat.com/ubi9/python-312@sha256:5b0c67feb3f2e5637c6301ea22e2839d402dd71d88c3cec8d1fd278ce0f9e69a
WORKDIR /opt/app-root/src
COPY --chown=1001:0 requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY --chown=1001:0 *.py *.txt ./
COPY --chown=1001:0 data ./data
RUN python prepare_assets.py
USER 1001
EXPOSE 8080
CMD ["python", "platform_service.py"]
