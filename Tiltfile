load('ext://restart_process', 'docker_build_with_restart')

# --- Configuration ---
REGISTRY = 'ghcr.io/chaosplane-hq'

# --- CRD manifests ---
k8s_yaml(kustomize('config/crd/bases'))

# --- Operator ---
local_resource(
    'operator-compile',
    'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/operator ./cmd/operator/...',
    deps=['cmd/operator', 'internal', 'pkg', 'api'],
)

docker_build_with_restart(
    REGISTRY + '/operator',
    '.',
    entrypoint=['/app/operator'],
    dockerfile='Dockerfile.operator',
    only=['bin/operator'],
    live_update=[
        sync('bin/operator', '/app/operator'),
    ],
)

k8s_yaml('config/manager/operator.yaml', allow_duplicates=True)
k8s_resource('operator', port_forwards='8080:8080')

# --- Agent ---
local_resource(
    'agent-compile',
    'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/agent ./cmd/agent/...',
    deps=['cmd/agent', 'internal', 'pkg'],
)

docker_build_with_restart(
    REGISTRY + '/agent',
    '.',
    entrypoint=['/app/agent'],
    dockerfile='Dockerfile.agent',
    only=['bin/agent'],
    live_update=[
        sync('bin/agent', '/app/agent'),
    ],
)

# --- Daemon (DaemonSet) ---
local_resource(
    'daemon-compile',
    'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/daemon ./cmd/daemon/...',
    deps=['cmd/daemon', 'internal', 'pkg'],
)

docker_build_with_restart(
    REGISTRY + '/daemon',
    '.',
    entrypoint=['/app/daemon'],
    dockerfile='Dockerfile.daemon',
    only=['bin/daemon'],
    live_update=[
        sync('bin/daemon', '/app/daemon'),
    ],
)
