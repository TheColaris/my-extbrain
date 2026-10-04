# my-extbrain server —— 多阶段构建，产物为 distroless 单层镜像（目标 <30MB）
# 前端产物（dist/）由构建方预先产出后拷入 cmd/app/dist/（仓库不含构建产物）。

FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
# 国内网络构建传 --build-arg GOPROXY=https://goproxy.cn,direct
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 版本注入：容器内 server/CLI 的 --version 对齐 git tag（缺省 git 时回退 dev）
ARG VERSION=dev
RUN go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/extbrain-server ./cmd/app && \
    go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/extbrain ./cmd/extbrain

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/extbrain-server /usr/local/bin/extbrain-server
COPY --from=build /out/extbrain /usr/local/bin/extbrain
ENV LISTEN_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["extbrain-server"]
