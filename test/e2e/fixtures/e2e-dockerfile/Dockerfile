# Proves the dockerfile driver: SLD builds this image and routes to its port.
FROM python:3.13-slim
ARG MARKER=E2E-DOCKERFILE-OK
RUN mkdir /srv/site && echo "$MARKER" > /srv/site/index.html
WORKDIR /srv/site
EXPOSE 9000
CMD ["python", "-m", "http.server", "9000"]
