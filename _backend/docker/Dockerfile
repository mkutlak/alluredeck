ARG BUILD_DATE
ARG BUILD_VERSION
ARG BUILD_REF
ARG UID=1000
ARG GID=1000

###### Go Build Stage
FROM golang:1.25-alpine AS build_stage
WORKDIR /build
COPY allure-docker-api/go.mod allure-docker-api/go.sum ./
RUN go mod download
COPY allure-docker-api/ ./
# Build the Go binary statically
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o allure-docker-api-bin ./cmd/api/

###### Final Image
# Use TARGETPLATFORM so cross-compilation builds the correct architecture image
FROM --platform=$TARGETPLATFORM node:25-alpine
ARG BUILD_DATE
ARG BUILD_VERSION
ARG BUILD_REF
ARG UID=1000
ARG GID=1000

LABEL org.label-schema.build-date=${BUILD_DATE} \
    org.label-schema.docker.dockerfile="docker/Dockerfile" \
    org.label-schema.license="MIT" \
    org.label-schema.name="Allure Framework" \
    org.label-schema.version=${BUILD_VERSION} \
    org.label-schema.description="Allure Framework is a flexible lightweight multi-language test report tool." \
    org.label-schema.url="https://docs.qameta.io/allure/" \
    org.label-schema.vcs-ref=${BUILD_REF} \
    org.label-schema.vcs-type="Git" \
    org.label-schema.vcs-url="https://github.com/mkutlak/allure-docker-service" \
    org.label-schema.arch=${TARGETPLATFORM}

# Pin allure to a specific major version to ensure reproducible builds.
# Update this pin deliberately when upgrading Allure.
ARG ALLURE_VERSION=3
RUN apk update && \
    apk add --no-cache \
    tzdata \
    curl && \
    npm install -g allure@${ALLURE_VERSION} && \
    mkdir -p /app

RUN deluser --remove-home node || true && \
    adduser -D -u ${UID} -h /home/allure allure && \
    chown -R ${UID}:${GID} /home/allure

ENV ROOT=/app
ENV ALLURE_RESOURCES=$ROOT/resources
ENV RESULTS_DIRECTORY=$ROOT/allure-results
ENV REPORT_DIRECTORY=$ROOT/allure-report
ENV RESULTS_HISTORY=$RESULTS_DIRECTORY/history
ENV REPORT_HISTORY=$REPORT_DIRECTORY/history
ENV ALLURE_VERSION=$ROOT/version
ENV EMAILABLE_REPORT_FILE_NAME='emailable-report-allure-docker-service.html'
ENV STATIC_CONTENT=$ROOT/allure-docker-api/static
ENV STATIC_CONTENT_PROJECTS=$STATIC_CONTENT/projects
ENV DEFAULT_PROJECT=default
ENV DEFAULT_PROJECT_ROOT=$STATIC_CONTENT_PROJECTS/$DEFAULT_PROJECT
ENV DEFAULT_PROJECT_RESULTS=$DEFAULT_PROJECT_ROOT/results
ENV DEFAULT_PROJECT_REPORTS=$DEFAULT_PROJECT_ROOT/reports
ENV EXECUTOR_FILENAME=executor.json
ENV ALLURE_NO_ANALYTICS=1
ENV CI=true

RUN echo -n $(allure --version) > ${ALLURE_VERSION} && \
    echo "ALLURE_VERSION: "$(cat ${ALLURE_VERSION}) && \
    ln -s $STATIC_CONTENT_PROJECTS $ROOT/projects && \
    ln -s $DEFAULT_PROJECT_REPORTS $ROOT/default-reports

WORKDIR $ROOT

# Copy the compiled Go binary from the build stage
COPY --from=build_stage --chown=${UID}:${GID} /build/allure-docker-api-bin $ROOT/allure-docker-api-bin

# Copy templates and static contents (needed by the API runtime depending on setup)
COPY --chown=${UID}:${GID} allure-docker-api/static $STATIC_CONTENT
COPY --chown=${UID}:${GID} allure-docker-api/templates $ROOT/allure-docker-api/templates

RUN mkdir $RESULTS_DIRECTORY && \
    mkdir -p $DEFAULT_PROJECT_REPORTS/latest && \
    ln -sf $RESULTS_DIRECTORY $DEFAULT_PROJECT_RESULTS && \
    ln -sf $DEFAULT_PROJECT_REPORTS/latest $REPORT_DIRECTORY

RUN chown -R ${UID}:${GID} $ROOT

VOLUME [ "$RESULTS_DIRECTORY" ]

ENV PORT=5050

EXPOSE $PORT

HEALTHCHECK --interval=10s --timeout=60s --retries=3 \
    CMD curl -f http://localhost:$PORT/version || exit 1

USER allure

CMD ["/app/allure-docker-api-bin"]
