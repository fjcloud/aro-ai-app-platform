FROM registry.access.redhat.com/hi/python:3.12

COPY terminal_server.py index.html /opt/terminal/
COPY vendor/ /opt/terminal/vendor/

ENV HOST=0.0.0.0 \
    PORT=8080 \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    HOME=/tmp

WORKDIR /opt/terminal
USER 65532
EXPOSE 8080

CMD ["python3.12", "/opt/terminal/terminal_server.py"]
