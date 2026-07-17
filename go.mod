module github.com/apstndb/spanner-mycli

go 1.25.12

require (
	cloud.google.com/go v0.123.0
	cloud.google.com/go/bigquery v1.79.0
	cloud.google.com/go/longrunning v1.2.0
	cloud.google.com/go/spanner v1.92.0
	cloud.google.com/go/storage v1.63.1
	github.com/MakeNowJust/heredoc/v2 v2.0.1
	github.com/alecthomas/kong v1.15.0
	github.com/apstndb/adcplus v0.2.0
	github.com/apstndb/developerknowledge-go v0.3.0
	github.com/apstndb/genaischema v0.2.0
	github.com/apstndb/go-grpcinterceptors v0.0.0-20241120095005-f07edaf5bdfe
	github.com/apstndb/go-tabwrap v0.1.4
	github.com/apstndb/gsqlutils v0.0.0-20260502161854-d7d6011a36e0
	github.com/apstndb/memebridge v0.7.0
	github.com/apstndb/protoyaml v0.1.1
	github.com/apstndb/spancodec v0.1.2
	github.com/apstndb/spanemuboost v0.4.6
	github.com/apstndb/spaniter v0.3.1
	github.com/apstndb/spanner-docs-embed v0.0.0-20260312161525-0136df2da2a6
	github.com/apstndb/spannerplan v0.2.1
	github.com/apstndb/spanstats v0.1.0
	github.com/apstndb/spantype v0.3.13
	github.com/apstndb/spanvalue v0.8.3
	github.com/bufbuild/protocompile v0.14.1
	github.com/cloudspannerecosystem/memefish v0.8.0
	github.com/creack/pty v1.1.24
	github.com/fatih/color v1.19.0
	github.com/go-sprout/sprout v1.0.3
	github.com/goccy/go-yaml v1.19.2
	github.com/gocql/gocql v1.7.0
	github.com/google/go-cmp v0.7.0
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/googleapis/go-spanner-cassandra v0.7.0
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.3
	github.com/hymkor/go-multiline-ny v0.23.1
	github.com/junegunn/fzf v0.74.0
	github.com/k0kubun/pp/v3 v3.5.1
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/klauspost/compress v1.19.0
	github.com/modelcontextprotocol/go-sdk v1.6.1
	github.com/nyaosorg/go-readline-ny v1.15.1
	github.com/olekukonko/tablewriter v1.1.4
	github.com/pelletier/go-toml v1.9.5
	github.com/samber/lo v1.53.0
	github.com/sourcegraph/conc v0.3.0
	github.com/spf13/afero v1.15.0
	github.com/stretchr/testify v1.11.1
	github.com/testcontainers/testcontainers-go v0.43.0
	github.com/vbauerster/mpb/v8 v8.12.1
	go.uber.org/zap v1.28.0
	golang.org/x/term v0.45.0
	golang.org/x/time v0.15.0
	google.golang.org/api v0.288.0
	google.golang.org/genai v1.63.0
	google.golang.org/grpc v1.82.0
	google.golang.org/protobuf v1.36.11
)

require (
	cel.dev/expr v0.25.1 // indirect
	cloud.google.com/go/auth v0.20.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	cloud.google.com/go/iam v1.11.0 // indirect
	cloud.google.com/go/monitoring v1.29.0 // indirect
	dario.cat/mergo v1.0.2 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c // indirect
	github.com/GoogleCloudPlatform/grpc-gcp-go/grpcgcp v1.6.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.32.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.57.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.57.0 // indirect
	github.com/Masterminds/semver/v3 v3.4.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/VividCortex/ewma v1.2.0 // indirect
	github.com/acarl005/stripansi v0.0.0-20180116102854-5a71ef0e047d // indirect
	github.com/apache/arrow/go/v15 v15.0.2 // indirect
	github.com/apstndb/gh-dev-tools v0.0.0-20250726094924-b386827f58e6 // indirect
	github.com/apstndb/github-schema-go v0.0.0-20250623031417-7b63713e7a90 // indirect
	github.com/apstndb/go-jq-yamlformat v0.0.0-20250724144043-044ee62273ff // indirect
	github.com/apstndb/go-yamlformat v0.0.0-20250624144133-5961930dd0ba // indirect
	github.com/apstndb/ptyhelp v0.2.3 // indirect
	github.com/apstndb/structfields v0.1.0 // indirect
	github.com/apstndb/structfields/spannertag v0.1.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charlievieth/fastwalk v1.0.14 // indirect
	github.com/charmbracelet/x/conpty v0.2.0 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/cncf/xds/go v0.0.0-20260202195803-dba9d589def2 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/platforms v0.2.1 // indirect
	github.com/cpuguy83/dockercfg v0.3.2 // indirect
	github.com/datastax/go-cassandra-native-protocol v0.0.0-20240903140133-605a850e203b // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/dmarkham/enumer v1.5.11 // indirect
	github.com/docker/go-connections v0.6.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/ebitengine/purego v0.10.0 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.37.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.3.3 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/gdamore/tcell/v2 v2.9.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/flatbuffers v23.5.26+incompatible // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.17 // indirect
	github.com/googleapis/gax-go/v2 v2.23.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/hailocab/go-hostpool v0.0.0-20160125115350-e80d13ce29ed // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/itchyny/gojq v0.12.17 // indirect
	github.com/itchyny/timefmt-go v0.1.6 // indirect
	github.com/junegunn/go-shellwords v0.0.0-20250127100254-2aa3b3277741 // indirect
	github.com/klauspost/cpuid/v2 v2.2.5 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20250821153705-5981dea3221d // indirect
	github.com/magiconair/properties v1.8.10 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/mattn/go-runewidth v0.0.23 // indirect
	github.com/mattn/go-tty v0.0.7 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/go-archive v0.2.0 // indirect
	github.com/moby/moby/api v1.54.2 // indirect
	github.com/moby/moby/client v0.4.0 // indirect
	github.com/moby/patternmatcher v0.6.1 // indirect
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/moby/sys/user v0.4.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/nyaosorg/go-ttyadapter v0.6.2 // indirect
	github.com/olekukonko/cat v0.0.0-20250911104152-50322a0618f6 // indirect
	github.com/olekukonko/errors v1.2.0 // indirect
	github.com/olekukonko/ll v0.1.6 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/pascaldekloe/name v1.0.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.18 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/shirou/gopsutil/v4 v4.26.5 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/cobra v1.9.1 // indirect
	github.com/spf13/pflag v1.0.7 // indirect
	github.com/spiffe/go-spiffe/v2 v2.6.0 // indirect
	github.com/testcontainers/testcontainers-go/modules/gcloud v0.42.0 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	github.com/zeebo/xxh3 v1.0.2 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.43.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.68.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.67.0 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/mod v0.36.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/telemetry v0.0.0-20260508192327-42602be52be6 // indirect
	golang.org/x/text v0.38.0 // indirect
	golang.org/x/tools v0.45.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	google.golang.org/genproto v0.0.0-20260519071638-aa98bba5eb94 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260630182238-925bb5da69e7 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260630182238-925bb5da69e7 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

tool (
	github.com/apstndb/gh-dev-tools/gh-helper
	github.com/apstndb/github-schema-go/cmd/github-schema
	github.com/apstndb/ptyhelp
	github.com/dmarkham/enumer
)
