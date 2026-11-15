# lookr scan Implementation Plan

> **Implemented and verified on 2026-11-15.** All 13 tasks completed. Final review added `--no-metrics` (independent paid CloudWatch opt-out), node-specific ElastiCache metrics, Multi-Attach-safe EBS rules, targetless ALB activity checks, stopped-RDS storage-only estimates, preserved partial inventories and missing security facts. These corrections supersede conflicting original details below. See `docs/superpowers/reports/2026-11-15-scan-verification.md` for validation and remaining nonblocking limits.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Substituir os 14 comandos do `lookr` por `lookr scan`, que varre uma conta AWS e devolve, por recurso, configuração, veredito de uso, custo mensal e achados de configuração, em tabela, JSON ou CSV.

**Architecture:** Cada tipo de recurso tem um coletor (`collect`), uma regra de uso (`usage`), um estimador de custo (`cost`) e verificações de configuração (`checks`), todos ligados pelo nome do tipo e pelas chaves de `Resource.Config`. O pacote `aws` orquestra: regiões em paralelo, métricas do CloudWatch em lote, custo real com queda para estimativa, erros parciais viram avisos. `output` grava o relatório.

**Tech Stack:** Go 1.25, AWS SDK for Go v2, cobra, `text/tabwriter`, `encoding/csv`, `encoding/json`. Testes com `testing` da biblioteca padrão e fakes escritos à mão.

**Spec:** `docs/superpowers/specs/2026-11-14-scan-inventario-uso-custo-design.md`

## Global Constraints

- Somente leitura: nenhuma chamada de API que crie, altere ou apague. Só `Describe*`, `List*`, `Get*`.
- Módulo Go continua `lookr`. `go.mod` passa para `go 1.25`.
- Sai `github.com/aws/aws-sdk-go` (v1), `github.com/olekukonko/tablewriter` e o pacote `deps/`. A alteração não commitada em `deps/regions.go` é descartada junto com o arquivo.
- Saída do CLI e campos do JSON em inglês. `schema_version` é `"1"`.
- Vereditos: `in_use`, `idle`, `orphan`, `unknown`, `not_evaluated`. Origem de custo: `actual`, `estimate`, `none`. Gravidade: `high`, `medium`, `low`.
- Na dúvida o veredito é `unknown`, nunca `idle`.
- Dados em stdout; progresso e avisos em stderr.
- Nenhum teste chama a AWS. Clientes do SDK ficam atrás de interfaces não exportadas, declaradas no arquivo que as usa, só com os métodos usados.
- Toda listagem pagina até o fim.
- Nenhum coletor pode dar panic com campo opcional nulo na resposta da AWS; usar `aws.ToString`, `aws.ToInt32` etc.
- Trabalhar no branch `feat/scan`. Um commit por tarefa, mensagem no estilo do repo (`feat: …`, `chore: …`).
- Em cada tarefa, `go build ./... && go vet ./... && go test ./...` precisa passar antes do commit.

## Review Focus

1. **Contador sem pontos no CloudWatch.** Lambda, load balancer, SQS e DynamoDB não publicam pontos quando não há atividade. Consulta bem-sucedida sem pontos em métrica de contador vale zero; consulta que falhou vale `unknown`. Teste na Task 4.
2. **Campos nulos na resposta da AWS.** Instância sem IP público, sem instance profile, sem security group, volume sem tags. Cada coletor tem um caso de teste com item esparso (Tasks 5 a 7).
3. **Falha parcial.** Região ou serviço com `AccessDenied` gera aviso e a varredura segue; Ctrl-C (contexto cancelado) encerra sem esperar todas as regiões. Testes na Task 11.
4. **Cost Explorer indisponível.** Não ativado ou sem permissão em modo `auto`: cai para estimativa com um único aviso, sem abortar. Testes nas Tasks 9 e 11.
5. **Argumentos inválidos.** `--type` desconhecido, `--output` ou `--cost` fora da lista, `--days 0`: erro claro listando os valores aceitos, código de saída 1, nenhuma chamada à AWS. Teste na Task 12.

## Estrutura de arquivos

```
main.go                                   chama cmd.Execute
cmd/root.go                               comando raiz
cmd/scan.go                               flags, validação, escolha da saída
internal/model/model.go                   Resource, Usage, Cost, Finding, Report
internal/model/report.go                  NewReport, ordenação, resumo
internal/provider/provider.go             interface Provider, ScanOptions
internal/provider/aws/scan.go             Deps, Run (orquestração)
internal/provider/aws/provider.go         New: liga Deps aos clientes reais
internal/provider/aws/collect/collect.go  Collector, Scope, All, Shorts
internal/provider/aws/collect/<tipo>.go   um arquivo por serviço
internal/provider/aws/usage/usage.go      Query, Metrics, Rule, Apply, QueriesFor
internal/provider/aws/usage/thresholds.go limiares com nome
internal/provider/aws/usage/rules.go      regras por tipo
internal/provider/aws/usage/cloudwatch.go Fetch
internal/provider/aws/cost/estimate.go    Estimate, estimadores por tipo, constantes
internal/provider/aws/cost/pricing.go     PriceLookup sobre a Pricing API
internal/provider/aws/cost/actual.go      FetchActual sobre o Cost Explorer
internal/provider/aws/checks/checks.go    Run e verificações por tipo
internal/output/{json,csv,table}.go       gravadores
internal/output/testdata/                 arquivos de referência
```

## Contrato por tipo

É o que liga coletores, regras, estimadores e verificações. `short` é o valor aceito em `--type` e mostrado na coluna TYPE. Valores de `Config` são `string`, `bool`, `int`, `float64` ou `[]string`. Métricas: `chave = Namespace/Métrica/Estatística (dimensão)`; `Z` marca contador em que ausência de pontos vale zero.

| Tipo / short | Escopo | Chaves de `Config` | Métricas | Veredito (primeira condição que casar) | Estimativa mensal |
|---|---|---|---|---|---|
| `aws.ec2.instance` / `ec2` | região | `instance_type`, `state`, `platform`, `private_ip`, `public_ip`, `image_id`, `vpc_id`, `security_groups` ([]string), `iam_instance_profile`, `imds_v2_required` (bool) | `cpu_avg`=AWS/EC2 CPUUtilization Average, `cpu_max`=…Maximum, `net_in`=NetworkIn Sum Z, `net_out`=NetworkOut Sum Z (InstanceId) | `state=="stopped"` → idle "instance is stopped"; `state!="running"` → unknown "instance state is X"; sem `cpu_avg` → unknown "no CPU metrics"; `cpu_avg<2 && cpu_max<10 && (net_in+net_out)/days < 5 MiB` → idle "avg CPU 0.8% over 30d"; senão in_use "avg CPU 37.2% over 30d" | parada: 0, nota "stopped; attached volumes billed separately"; rodando: preço/hora on-demand × 730 |
| `aws.ebs.volume` / `ebs` | região | `size_gb`, `volume_type`, `iops`, `throughput`, `encrypted`, `state`, `attached_instance_id`, `attached_instance_state` | — | `state=="available"` → orphan "not attached to any instance"; `attached_instance_state=="stopped"` → idle "attached to stopped instance i-…"; senão in_use "attached to i-…" | `size_gb` × preço GB-mês do `volume_type`; nota "storage only; provisioned IOPS and throughput excluded" |
| `aws.ebs.snapshot` / `snapshot` | região | `volume_id`, `volume_size_gb`, `state`, `encrypted`, `description`, `storage_tier`, `source_volume_exists`, `used_by_ami` | — | `!source_volume_exists && !used_by_ami` e idade > `days` → orphan "source volume no longer exists"; senão not_evaluated "inventory only" | `volume_size_gb` × preço GB-mês de snapshot; nota "upper bound: assumes full volume size" |
| `aws.ec2.eip` / `eip` | região | `public_ip`, `allocation_id`, `association_id`, `instance_id`, `network_interface_id` | — | `association_id==""` → orphan "not associated"; senão in_use "associated" | constante `EIPHourlyUSD` × 730 |
| `aws.ec2.natgateway` / `nat` | região | `state`, `vpc_id`, `subnet_id`, `connectivity_type` | `bytes_out`=AWS/NATGateway BytesOutToDestination Sum Z, `bytes_in`=BytesInFromDestination Sum Z (NatGatewayId) | soma == 0 → idle "0 bytes over 30d"; senão in_use | preço/hora × 730; nota "data processing excluded" |
| `aws.ec2.securitygroup` / `sg` | região | `vpc_id`, `description`, `ingress_rule_count`, `world_open_ports` ([]string, ex. `"22"`, `"all"`) | — | sem regra (not_evaluated) | sem estimador |
| `aws.elb.loadbalancer` / `elb` | região | `lb_type` (application, network, gateway, classic), `scheme`, `dns_name`, `state`, `vpc_id`, `target_count`, `metric_dimension` | application: `activity`=AWS/ApplicationELB RequestCount Sum Z (LoadBalancer); network: `activity`=AWS/NetworkELB NewFlowCount Sum Z (LoadBalancer); classic: `activity`=AWS/ELB RequestCount Sum Z (LoadBalancerName); gateway: nenhuma | `lb_type=="gateway"` → unknown "no usage rule for gateway load balancers"; `target_count==0` → orphan "no registered targets"; `activity==0` → idle "0 requests over 30d"; senão in_use | preço/hora do tipo × 730; nota "capacity units excluded" |
| `aws.rds.instance` / `rds` | região | `engine`, `engine_version`, `instance_class`, `status`, `multi_az`, `storage_type`, `allocated_storage_gb`, `az`, `port`, `cluster_id`, `publicly_accessible`, `storage_encrypted`, `backup_retention_days` | `conn_max`=AWS/RDS DatabaseConnections Maximum (DBInstanceIdentifier) | `status=="stopped"` → idle "instance is stopped"; sem `conn_max` → unknown "no connection metrics"; `conn_max==0` → idle "0 connections over 30d"; senão in_use "max 14 connections" | preço/hora (classe, engine, Single-AZ ou Multi-AZ) × 730, mais `allocated_storage_gb` × preço GB-mês quando `cluster_id==""`; nota "backups and I/O excluded" |
| `aws.rds.cluster` / `aurora` | região | `engine`, `engine_version`, `status`, `member_count`, `engine_mode` | `conn_max`=AWS/RDS DatabaseConnections Maximum (DBClusterIdentifier) | `member_count==0` → orphan "cluster has no instances"; depois igual a `rds` | none, nota "instances are billed separately; cluster storage not estimated" |
| `aws.elasticache.cluster` / `elasticache` | região | `engine`, `engine_version`, `node_type`, `num_nodes`, `status`, `replication_group_id` | redis/valkey: `cmds_get`=AWS/ElastiCache GetTypeCmds Sum, `cmds_set`=SetTypeCmds Sum; memcached: `cmds_get`=CmdGet Sum, `cmds_set`=CmdSet Sum (CacheClusterId) | faltando qualquer uma → unknown "no command metrics"; soma == 0 → idle "0 commands over 30d"; senão in_use | preço/hora do nó × `num_nodes` × 730 |
| `aws.eks.cluster` / `eks` | região | `version`, `status`, `endpoint`, `nodegroup_count`, `fargate_profile_count`, `tagged_instance_count`, `endpoint_public`, `public_access_cidrs` ([]string) | — | os três contadores == 0 → idle "no node groups, Fargate profiles or tagged instances"; senão in_use | constante `EKSClusterHourlyUSD` × 730; nota "control plane only" |
| `aws.lambda.function` / `lambda` | região | `runtime`, `handler`, `memory_mb`, `timeout_s`, `architecture`, `code_size_bytes`, `last_modified` | `invocations`=AWS/Lambda Invocations Sum Z, `duration_ms`=Duration Sum Z (FunctionName) | `invocations==0` → idle "0 invocations over 30d"; senão in_use "N invocations over 30d" | (`invocations` × preço por requisição + `duration_ms`/1000 × `memory_mb`/1024 × preço GB-segundo) × 30/`days`; nota "free tier ignored" |
| `aws.dynamodb.table` / `dynamodb` | região | `status`, `billing_mode` (PROVISIONED, PAY_PER_REQUEST), `read_capacity`, `write_capacity`, `item_count`, `size_bytes`, `table_class` | `read_units`=AWS/DynamoDB ConsumedReadCapacityUnits Sum Z, `write_units`=ConsumedWriteCapacityUnits Sum Z (TableName) | soma == 0 → idle "0 read/write units over 30d"; senão in_use | PROVISIONED: (`read_capacity` × preço RCU-hora + `write_capacity` × preço WCU-hora) × 730; PAY_PER_REQUEST: none "no estimate for usage-based pricing" |
| `aws.sqs.queue` / `sqs` | região | `url`, `fifo`, `visibility_timeout_s`, `approximate_messages`, `retention_s`, `encrypted`, `has_dlq` | `sent`=AWS/SQS NumberOfMessagesSent Sum Z, `received`=NumberOfMessagesReceived Sum Z (QueueName) | soma == 0 → idle "0 messages sent or received over 30d"; senão in_use | none "no estimate for usage-based pricing" |
| `aws.cloudfront.distribution` / `cloudfront` | global | `domain_name`, `status`, `enabled`, `aliases` ([]string), `price_class`, `viewer_protocol_policy`, `min_tls_version` | `requests`=AWS/CloudFront Requests Sum Z (DistributionId, Region=Global), região da consulta `us-east-1` | `!enabled` → idle "distribution is disabled"; `requests==0` → idle "0 requests over 30d"; senão in_use | none "no estimate for usage-based pricing" |
| `aws.s3.bucket` / `s3` | global | `bucket_region`, `public_access_blocked` (bool) | `object_count`=AWS/S3 NumberOfObjects Average (BucketName, StorageType=AllStorageTypes), `size_bytes`=BucketSizeBytes Average (BucketName, StorageType=StandardStorage), região da consulta `bucket_region` | sem `object_count` → unknown "no storage metrics"; `object_count==0` → idle "bucket is empty"; senão unknown "S3 publishes no access metrics by default" | `size_bytes`/GiB × preço GB-mês Standard; nota "Standard storage class only"; sem `size_bytes` → none |
| `aws.iam.user` / `iam` | global | `path`, `password_last_used`, `has_console_password`, `mfa_enabled`, `oldest_active_key_age_days` (int, -1 sem chave) | — | sem regra | sem estimador |
| `aws.iam.group`, `aws.iam.role` / `iam` | global | `path`; role também `last_used` | — | sem regra | sem estimador |
| `aws.acm.certificate` / `acm` | região | `domain_name`, `status`, `cert_type`, `in_use_count`, `not_after` (RFC 3339) | — | sem regra | sem estimador |
| `aws.route53.hostedzone` / `route53` | global | `private`, `record_count` | — | sem regra | constante `Route53ZoneMonthlyUSD`; nota "hosted zone fee only" |

`ID` é o identificador nativo (instance id, volume id, nome da função, nome do bucket). `ARN` preenchido sempre que o serviço fornecer ou der para montar. `Name` vem da tag `Name` quando houver, senão do nome nativo. Recursos globais usam `Region: "global"`.

---

### Task 1: Base do módulo, modelo e relatório

**Files:**
- Delete: `cmd/aws_*.go`, `deps/regions.go`
- Modify: `go.mod`, `main.go`, `cmd/root.go`
- Create: `internal/model/model.go`, `internal/model/report.go`
- Test: `internal/model/report_test.go`, `internal/model/model_test.go`

**Interfaces:**
- Produces:

```go
package model

type Verdict string    // VerdictInUse, VerdictIdle, VerdictOrphan, VerdictUnknown, VerdictNotEvaluated
type CostSource string // CostActual, CostEstimate, CostNone
type Severity string   // SeverityHigh, SeverityMedium, SeverityLow

type Usage struct {
    Verdict    Verdict            `json:"verdict"`
    Reason     string             `json:"reason"`
    WindowDays int                `json:"window_days"`
    Metrics    map[string]float64 `json:"metrics"`
}
type Cost struct {
    MonthlyUSD *float64   `json:"monthly_usd"`
    Source     CostSource `json:"source"`
    Note       string     `json:"note"`
}
type Finding struct {
    Check          string   `json:"check"`
    Severity       Severity `json:"severity"`
    Message        string   `json:"message"`
    Recommendation string   `json:"recommendation"`
}
type Resource struct {
    Provider  string            `json:"provider"`
    AccountID string            `json:"account_id"`
    Region    string            `json:"region"`
    Type      string            `json:"type"`
    Short     string            `json:"-"`
    ID        string            `json:"id"`
    ARN       string            `json:"arn"`
    Name      string            `json:"name"`
    Tags      map[string]string `json:"tags"`
    CreatedAt *time.Time        `json:"created_at"`
    Config    map[string]any    `json:"config"`
    Usage     Usage             `json:"usage"`
    Cost      Cost              `json:"cost"`
    Findings  []Finding         `json:"findings"`
}
func (r Resource) Key() string            // Type + "|" + Region + "|" + ID
func (r Resource) Str(key string) string  // "" se ausente ou de outro tipo
func (r Resource) Int(key string) int     // aceita int, int32, int64, float64; 0 se ausente
func (r Resource) Float(key string) float64
func (r Resource) Bool(key string) bool
func (r Resource) Strs(key string) []string

type Warning struct {
    Region string `json:"region"`
    Type   string `json:"type"`
    Error  string `json:"error"`
}
type Summary struct {
    Resources          int              `json:"resources"`
    MonthlyUSD         float64          `json:"monthly_usd"`
    WasteResources     int              `json:"waste_resources"`
    WasteMonthlyUSD    float64          `json:"waste_monthly_usd"`
    ByVerdict          map[Verdict]int  `json:"by_verdict"`
    FindingsBySeverity map[Severity]int `json:"findings_by_severity"`
}
type Report struct {
    SchemaVersion string     `json:"schema_version"`
    GeneratedAt   time.Time  `json:"generated_at"`
    Provider      string     `json:"provider"`
    AccountID     string     `json:"account_id"`
    WindowDays    int        `json:"window_days"`
    Regions       []string   `json:"regions"`
    Summary       Summary    `json:"summary"`
    Resources     []Resource `json:"resources"`
    Warnings      []Warning  `json:"warnings"`
}
func NewReport(provider, accountID string, windowDays int, regions []string, resources []Resource, warnings []Warning, now time.Time) Report
```

- [x] **Step 1: Criar o branch e limpar o que sai**

```bash
git checkout -b feat/scan
git rm -q cmd/aws_*.go && git rm -qf deps/regions.go
```

Reduzir `main.go` a `cmd.Execute()` com saída 1 em erro (mensagem em stderr); `cmd/root.go` mantém só o `rootCmd` com `Use: "lookr"`, `SilenceUsage: true`. Em `go.mod`: `go 1.25`; depois `go mod tidy` (remove SDK v1 e tablewriter).

- [x] **Step 2: Escrever os testes que falham**

`report_test.go`:

```go
func TestNewReportSummary(t *testing.T) {
    res := []Resource{
        {Type: "aws.ec2.instance", ID: "i-1", Usage: Usage{Verdict: VerdictInUse}, Cost: Cost{MonthlyUSD: ptr(100), Source: CostActual}},
        {Type: "aws.ebs.volume", ID: "vol-1", Usage: Usage{Verdict: VerdictOrphan}, Cost: Cost{MonthlyUSD: ptr(80), Source: CostEstimate},
            Findings: []Finding{{Check: "ebs.unencrypted", Severity: SeverityMedium}}},
        {Type: "aws.elb.loadbalancer", ID: "lb-1", Usage: Usage{Verdict: VerdictIdle}, Cost: Cost{MonthlyUSD: ptr(18.4), Source: CostEstimate}},
        {Type: "aws.iam.role", ID: "r-1", Usage: Usage{Verdict: VerdictNotEvaluated}, Cost: Cost{Source: CostNone}},
    }
    r := NewReport("aws", "123456789012", 30, []string{"us-east-1"}, res, nil, fixedNow)
    // SchemaVersion "1"; Summary.Resources 4; MonthlyUSD 198.4; WasteResources 2; WasteMonthlyUSD 98.4
    // ByVerdict tem as cinco chaves (as ausentes com 0); FindingsBySeverity tem as três chaves, medium == 1
    // Warnings != nil e len 0; Resources[i].Findings, Tags, Config e Usage.Metrics nunca nil
}

func TestNewReportOrder(t *testing.T) {
    // ordem dos vereditos: orphan, idle, unknown, in_use, not_evaluated;
    // dentro do veredito: custo decrescente, sem custo por último; empate: Type, depois ID
}
```

`model_test.go`: `TestAccessors` cobre `Int` com `int`, `int32`, `int64`, `float64`, chave ausente e tipo errado; `Str`, `Bool`, `Strs` com ausente e tipo errado; `Key()` == `"aws.ebs.volume|us-east-1|vol-1"`.

- [x] **Step 3: Rodar e ver falhar**

Run: `go test ./internal/model/...` — Expected: FAIL (símbolos indefinidos).

- [x] **Step 4: Implementar `model.go` e `report.go`**

`NewReport` não altera a fatia recebida: copia, normaliza mapas e fatias nulos para vazios, ordena, calcula o resumo. Valores monetários do resumo arredondados a 2 casas.

- [x] **Step 5: Verificar e commitar**

Run: `go build ./... && go vet ./... && go test ./...` — Expected: PASS.

```bash
git add -A && git commit -m "feat: add scan data model and drop per-service commands"
```

---

### Task 2: Saídas JSON, CSV e tabela

**Files:**
- Create: `internal/output/json.go`, `internal/output/csv.go`, `internal/output/table.go`
- Create: `internal/output/testdata/report.json`, `report.csv`, `report.txt`
- Test: `internal/output/output_test.go`

**Interfaces:**
- Consumes: `model.Report` (Task 1).
- Produces: `func WriteJSON(w io.Writer, r model.Report) error`, `func WriteCSV(w io.Writer, r model.Report) error`, `func WriteTable(w io.Writer, r model.Report) error`.

- [x] **Step 1: Escrever os testes que falham**

Um relatório de exemplo construído em `sampleReport()` com: volume órfão estimado em 80.00 com um achado medium; instância ociosa `i-04f2` com nome `api-2` e custo real 61.32; RDS em uso com um achado high; role IAM sem custo; um recurso com nome `a,"b"` e tag com quebra de linha; um aviso.

```go
func TestWriteJSONGolden(t *testing.T)  // compara com testdata/report.json; indentação de 2 espaços; termina em "\n"
func TestWriteCSVGolden(t *testing.T)   // compara com testdata/report.csv
func TestWriteTableGolden(t *testing.T) // compara com testdata/report.txt
func TestWriteCSVRoundTrip(t *testing.T) // encoding/csv lê de volta: len(records) == len(Resources)+1, todos com 17 campos
func TestWriteTableEmpty(t *testing.T)   // relatório sem recursos: imprime cabeçalho e "No resources found.", sem panic nem divisão por zero
```

Flag `-update` no teste regrava os arquivos de referência.

- [x] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/output/...` — Expected: FAIL.

- [x] **Step 3: Implementar os três gravadores**

CSV, cabeçalho exato:
`provider,account_id,region,type,id,arn,name,created_at,verdict,reason,monthly_usd,cost_source,cost_note,tags,config,metrics,findings`.
`created_at` em RFC 3339 ou vazio; `monthly_usd` com 2 casas ou vazio; as quatro últimas colunas são JSON compacto.

Tabela, com `text/tabwriter` (o bloco abaixo mostra o formato; os números vêm de `sampleReport()`):

```
Account 123456789012 · 1 region · 5 resources · $339.72/mo
Idle or orphan: 2 resources · $141.32/mo (42%)
Findings: 1 high · 1 medium · 0 low
Warnings: 1 (see stderr)

TYPE  RESOURCE        REGION     VERDICT  REASON                 COST/MO          ISSUES
ebs   vol-0a1b2c3d    us-east-1  orphan   not attached to …      $80.00 (est.)    1

FINDINGS
HIGH    rds  db-homolog  us-east-1  database is publicly accessible
        -> Disable public accessibility or restrict the security group to known addresses
```

Regras: `1 region`/`N regions`; valores acima de mil com separador (`$3,482.10`); RESOURCE é `ID`, ou `ID (Name)` quando `Name` não é vazio nem igual ao `ID`, cortado em 40 caracteres com `…`; REASON cortado em 40; custo `$80.00 (est.)`, `$61.32 (actual)` ou `-`; ISSUES é a contagem ou `-`; linha `Warnings` só aparece se houver; bloco `FINDINGS` só aparece se houver, ordenado por gravidade e depois pela ordem dos recursos; percentual é 0 quando o total é 0.

- [x] **Step 4: Gerar referências, conferir à mão e rodar**

Run: `go test ./internal/output/... -update && git diff --stat internal/output/testdata` e ler os três arquivos gerados; depois `go test ./...` — Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add json, csv and table writers"
```

---

### Task 3: Regras de uso

**Files:**
- Create: `internal/provider/aws/usage/usage.go`, `thresholds.go`, `rules.go`
- Test: `internal/provider/aws/usage/usage_test.go`, `rules_test.go`

**Interfaces:**
- Consumes: `model` (Task 1); chaves de `Config` e métricas do Contrato por tipo.
- Produces:

```go
package usage

type Query struct {
    Key           string            // chave em Metrics, ex. "cpu_avg"
    Namespace     string
    Metric        string
    Stat          string            // "Sum", "Average", "Maximum"
    Dimensions    map[string]string
    ZeroIfMissing bool
    Region        string            // vazio = região do recurso
}
type Metrics map[string]float64
type Result struct {
    Metrics Metrics
    Err     error
}
type Rule struct {
    Queries  func(r model.Resource) []Query // pode ser nil
    Evaluate func(r model.Resource, m Metrics, now time.Time, days int) (model.Verdict, string)
}
func Rules() map[string]Rule                        // por nome de tipo
func QueriesFor(r model.Resource) []Query           // nil se o tipo não tem regra ou não consulta métricas
func Apply(r *model.Resource, res Result, now time.Time, days int)
```

Constantes em `thresholds.go`: `MinAgeDays = 7`, `EC2IdleCPUAvg = 2.0`, `EC2IdleCPUMax = 10.0`, `EC2IdleNetBytesPerDay = 5 << 20`.

- [x] **Step 1: Escrever os testes que falham**

`usage_test.go`, ordem de decisão do `Apply`:

```go
func TestApplyNoRule(t *testing.T)        // aws.iam.role → not_evaluated, "inventory only"
func TestApplyTooNew(t *testing.T)        // volume "available" criado há 6 dias → unknown, "too new to evaluate"; há 7 dias → orphan
func TestApplyMetricsError(t *testing.T)  // lambda com res.Err → unknown, "metrics unavailable: <erro>"
func TestApplyMetricsErrorIgnoredWithoutQueries(t *testing.T) // volume com res.Err → ainda orphan
func TestApplySetsWindowAndMetrics(t *testing.T) // WindowDays == days; Usage.Metrics == res.Metrics, nunca nil
func TestApplyNilCreatedAt(t *testing.T)  // CreatedAt nil não dá panic e segue para a regra
```

`rules_test.go`: uma tabela `[]struct{name string; r model.Resource; m Metrics; want model.Verdict; reason string}` com uma linha por condição da coluna "Veredito" do Contrato por tipo, usando os motivos exatos de lá, mais as bordas: EC2 com `cpu_avg` 1.99 e `cpu_max` 10.0 → in_use; EC2 com CPU baixa e rede de 6 MiB/dia → in_use; ElastiCache memcached usa as mesmas chaves `cmds_get`/`cmds_set`; snapshot órfão com idade igual a `days` → not_evaluated; S3 com `object_count` 0 → idle e 12 → unknown.

`TestQueriesFor`: ALB devolve dimensão `LoadBalancer` com o valor de `metric_dimension`; classic usa `LoadBalancerName`; gateway devolve nil; CloudFront devolve `Region: "us-east-1"` e dimensões `DistributionId` e `Region=Global`; S3 devolve `Region` igual a `bucket_region`; todo tipo com coluna "Métricas" preenchida devolve as chaves listadas, com `ZeroIfMissing` igual ao `Z` do contrato.

`TestRulesCoverContract`: todo tipo do contrato com veredito definido tem entrada em `Rules()`; `sg`, `iam.*`, `acm`, `route53` não têm.

- [x] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/provider/aws/usage/...` — Expected: FAIL.

- [x] **Step 3: Implementar**

`Apply`, nesta ordem: sem regra → not_evaluated; `CreatedAt` a menos de `MinAgeDays` → unknown; regra com consultas e `res.Err != nil` → unknown; senão `Evaluate`. Motivos formatam a janela como `over %dd` e percentuais com uma casa.

- [x] **Step 4: Verificar e commitar**

Run: `go build ./... && go vet ./... && go test ./...` — Expected: PASS.

```bash
git add -A && git commit -m "feat: add usage rules per resource type"
```

---

### Task 4: Métricas do CloudWatch

**Files:**
- Create: `internal/provider/aws/usage/cloudwatch.go`
- Test: `internal/provider/aws/usage/cloudwatch_test.go`

**Interfaces:**
- Consumes: `Query`, `Metrics`, `Result` (Task 3).
- Produces:

```go
type Request struct {
    ResourceKey string // model.Resource.Key()
    Queries     []Query
}
// Todas as requisições de uma chamada são da mesma região.
func Fetch(ctx context.Context, api cloudWatchAPI, reqs []Request, start, end time.Time) map[string]Result
func NewCloudWatch(cfg aws.Config) cloudWatchAPI
```

`cloudWatchAPI` tem só `GetMetricData`.

- [x] **Step 1: Escrever os testes que falham**

Com um fake que grava as entradas e devolve respostas programadas:

```go
func TestFetchAggregates(t *testing.T)
// Sum [1,2,3] → 6; Maximum [4,9,2] → 9; Average [10,20] → 15
func TestFetchMissingDatapoints(t *testing.T)
// consulta sem pontos: ZeroIfMissing → chave presente com 0; sem ZeroIfMissing → chave ausente; Err nil nos dois
func TestFetchBatches(t *testing.T)
// 1.201 consultas → 3 chamadas, nenhuma com mais de 500 MetricDataQueries; todas as chaves de recurso no resultado
func TestFetchPaginates(t *testing.T)
// NextToken na primeira resposta → segunda chamada com o token; pontos das duas páginas somados
func TestFetchBatchError(t *testing.T)
// erro no 2º lote → recursos desse lote com Err != nil; recursos do 1º lote com Err nil e métricas
func TestFetchRequestShape(t *testing.T)
// Period 86400; StartTime/EndTime repassados; Ids casam ^[a-z][a-zA-Z0-9_]*$ e são únicos na chamada; ReturnData true
func TestFetchEmpty(t *testing.T)
// reqs vazio → mapa vazio, nenhuma chamada
```

- [x] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/provider/aws/usage/... -run Fetch` — Expected: FAIL.

- [x] **Step 3: Implementar `Fetch`**

Ids `q0`, `q1`, … mapeados de volta para (recurso, chave). Um recurso cujas consultas caem em lotes diferentes recebe `Err` se qualquer um dos lotes falhar. Agregação feita no cliente sobre os pontos diários.

- [x] **Step 4: Verificar e commitar**

Run: `go build ./... && go vet ./... && go test ./...` — Expected: PASS.

```bash
git add -A && git commit -m "feat: fetch usage metrics from CloudWatch in batches"
```

---

### Task 5: Coletores de computação e rede

**Files:**
- Create: `internal/provider/aws/collect/collect.go`, `ec2.go` (instance, eip, natgateway, securitygroup), `ebs.go` (volume, snapshot), `elb.go`
- Test: `internal/provider/aws/collect/{collect,ec2,ebs,elb}_test.go`

**Interfaces:**
- Consumes: `model` (Task 1); Contrato por tipo.
- Produces:

```go
package collect

type Scope struct {
    AccountID string
    Region    string // "global" para coletores globais
}
type Collector struct {
    Type    string // "aws.ec2.instance"
    Short   string // "ec2"
    Global  bool
    Collect func(ctx context.Context, cfg aws.Config, s Scope) ([]model.Resource, error)
}
func All() []Collector  // ordem fixa; cresce nas Tasks 6 e 7
func Shorts() []string  // valores distintos de Short, ordenados
```

Cada recurso sai com `Provider: "aws"`, `AccountID`, `Region`, `Type`, `Short`, `ID`, `ARN`, `Name`, `Tags`, `CreatedAt` (quando o serviço informa) e `Config` conforme o contrato. `Usage`, `Cost` e `Findings` ficam vazios.

- [x] **Step 1: Escrever os testes que falham**

Para cada coletor, uma função interna `collectX(ctx, api, scope)` testada com fake. Cada teste entrega duas páginas e um item esparso (só os campos obrigatórios) e confere quantidade, campos de identidade e as chaves de `Config` do contrato:

```go
func TestCollectInstances(t *testing.T)
// 2 páginas → 3 recursos; instâncias "terminated" são puladas; item sem PublicIpAddress, IamInstanceProfile,
// SecurityGroups e Tags não dá panic e sai com public_ip "" e security_groups vazio;
// Name vem da tag Name; ARN == arn:aws:ec2:<região>:<conta>:instance/<id>; imds_v2_required reflete HttpTokens == "required"
func TestCollectVolumes(t *testing.T)
// volume anexado recebe attached_instance_state de DescribeInstances; volume "available" sai com attached_instance_id ""
func TestCollectSnapshots(t *testing.T)
// OwnerIds == ["self"]; source_volume_exists falso quando o VolumeId não está em DescribeVolumes;
// used_by_ami verdadeiro quando o snapshot aparece em BlockDeviceMappings de DescribeImages(Owners=self)
func TestCollectAddresses(t *testing.T)   // endereço sem AssociationId → association_id ""
func TestCollectNatGateways(t *testing.T) // gateways em estado "deleted" são pulados
func TestCollectSecurityGroups(t *testing.T)
// regra tcp 22 de 0.0.0.0/0 → world_open_ports contém "22"; faixa 0-65535 ou protocolo -1 de ::/0 → "all";
// regra de 10.0.0.0/8 não entra
func TestCollectLoadBalancers(t *testing.T)
// ALB: metric_dimension == "app/nome/abc123" (sufixo do ARN depois de "loadbalancer/"); target_count soma os alvos
// registrados de todos os target groups; clássico: lb_type "classic", metric_dimension == nome, target_count == nº de instâncias
func TestAllUniqueTypes(t *testing.T)     // Type único em All(); Short não vazio
func TestCollectorPropagatesError(t *testing.T) // erro da API volta como erro, com recursos nil
```

- [x] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/provider/aws/collect/...` — Expected: FAIL.

- [x] **Step 3: Implementar**

`go get` dos módulos `config`, `service/ec2`, `service/elasticloadbalancing`, `service/elasticloadbalancingv2` do SDK v2. Usar os paginadores do SDK. O coletor de ELB reúne v2 e clássico no mesmo tipo.

- [x] **Step 4: Verificar e commitar**

Run: `go build ./... && go vet ./... && go test ./...` — Expected: PASS.

```bash
git add -A && git commit -m "feat: collect EC2, EBS, network and load balancer resources"
```

---

### Task 6: Coletores de dados

**Files:**
- Create: `internal/provider/aws/collect/rds.go` (instance, cluster), `elasticache.go`, `dynamodb.go`, `s3.go`, `sqs.go`
- Modify: `internal/provider/aws/collect/collect.go` (registrar em `All`)
- Test: um `_test.go` por arquivo

**Interfaces:**
- Consumes: `Collector`, `Scope` (Task 5); Contrato por tipo.
- Produces: coletores `aws.rds.instance`, `aws.rds.cluster`, `aws.elasticache.cluster`, `aws.dynamodb.table`, `aws.s3.bucket` (Global), `aws.sqs.queue`.

- [x] **Step 1: Escrever os testes que falham**

Mesmo padrão da Task 5 (duas páginas, item esparso, chaves do contrato), mais:

```go
func TestCollectDBInstances(t *testing.T)  // membro de cluster Aurora sai com cluster_id preenchido; item sem Endpoint → port 0
func TestCollectDBClusters(t *testing.T)   // member_count == len(DBClusterMembers)
func TestCollectCacheClusters(t *testing.T)
func TestCollectTables(t *testing.T)
// ListTables + DescribeTable; sem BillingModeSummary → billing_mode "PROVISIONED";
// erro em DescribeTable de uma tabela devolve erro (não descarta em silêncio)
func TestCollectBuckets(t *testing.T)
// bucket_region: LocationConstraint vazio → "us-east-1"; public_access_blocked verdadeiro só com as quatro opções ligadas;
// erro NoSuchPublicAccessBlockConfiguration → falso; AccessDenied em um bucket → bucket entra com bucket_region ""
// e não derruba os outros; Region do recurso == "global"
func TestCollectQueues(t *testing.T)
// atributo pedido é "QueueArn"; ID == nome da fila (último segmento da URL); fifo pelo sufixo ".fifo";
// encrypted verdadeiro com SqsManagedSseEnabled "true" ou KmsMasterKeyId não vazio; has_dlq por RedrivePolicy;
// atributo ausente no mapa não dá panic
```

- [x] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/provider/aws/collect/...` — Expected: FAIL.

- [x] **Step 3: Implementar** (`go get` de `service/rds`, `elasticache`, `dynamodb`, `s3`, `sqs`). No S3, as chamadas por bucket usam um cliente na região do bucket.

- [x] **Step 4: Verificar e commitar**

Run: `go build ./... && go vet ./... && go test ./...` — Expected: PASS.

```bash
git add -A && git commit -m "feat: collect RDS, ElastiCache, DynamoDB, S3 and SQS resources"
```

---

### Task 7: Coletores restantes

**Files:**
- Create: `internal/provider/aws/collect/lambda.go`, `eks.go`, `cloudfront.go`, `iam.go` (user, group, role), `acm.go`, `route53.go`
- Modify: `internal/provider/aws/collect/collect.go`
- Test: um `_test.go` por arquivo

**Interfaces:**
- Consumes: `Collector`, `Scope` (Task 5); Contrato por tipo.
- Produces: coletores `aws.lambda.function`, `aws.eks.cluster`, `aws.cloudfront.distribution` (Global), `aws.iam.user`, `aws.iam.group`, `aws.iam.role` (Global), `aws.acm.certificate`, `aws.route53.hostedzone` (Global). Com esta tarefa `Shorts()` devolve os 19 valores do contrato.

- [x] **Step 1: Escrever os testes que falham**

Mesmo padrão, mais:

```go
func TestCollectFunctions(t *testing.T) // architecture: primeira de Architectures, "x86_64" se vazio
func TestCollectClusters(t *testing.T)
// nodegroup_count e fargate_profile_count por listagem paginada; tagged_instance_count conta instâncias "running" ou "pending"
// com a tag-key kubernetes.io/cluster/<nome>; endpoint_public e public_access_cidrs de ResourcesVpcConfig
func TestCollectDistributions(t *testing.T)
// DistributionList nulo → zero recursos, sem panic; viewer_protocol_policy do DefaultCacheBehavior;
// min_tls_version de ViewerCertificate.MinimumProtocolVersion
func TestCollectUsers(t *testing.T)
// has_console_password falso quando GetLoginProfile devolve NoSuchEntity; mfa_enabled por ListMFADevices;
// oldest_active_key_age_days calculado só com chaves "Active", -1 sem nenhuma
func TestCollectRolesAndGroups(t *testing.T)
func TestCollectCertificates(t *testing.T) // in_use_count == len(InUseBy); not_after em RFC 3339, "" se nulo
func TestCollectHostedZones(t *testing.T)  // ID sem o prefixo "/hostedzone/"; Config nulo → private false
func TestShortsComplete(t *testing.T)
// Shorts() == acm, aurora, cloudfront, dynamodb, ebs, ec2, eip, eks, elasticache, elb, iam, lambda, nat, rds, route53, s3, sg, snapshot, sqs
```

O coletor de usuários recebe o relógio por parâmetro (`now time.Time` em `collectUsers`) para o teste da idade da chave.

- [x] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/provider/aws/collect/...` — Expected: FAIL.

- [x] **Step 3: Implementar** (`go get` de `service/lambda`, `eks`, `cloudfront`, `iam`, `acm`, `route53`).

- [x] **Step 4: Verificar e commitar**

Run: `go build ./... && go vet ./... && go test ./...` — Expected: PASS.

```bash
git add -A && git commit -m "feat: collect Lambda, EKS, CloudFront, IAM, ACM and Route53 resources"
```

---

### Task 8: Custo estimado

**Files:**
- Create: `internal/provider/aws/cost/estimate.go`, `pricing.go`
- Test: `internal/provider/aws/cost/estimate_test.go`, `pricing_test.go`

**Interfaces:**
- Consumes: `model` (Task 1), `usage.Metrics` (Task 3); coluna "Estimativa mensal" do contrato.
- Produces:

```go
package cost

const (
    HoursPerMonth         = 730.0
    EIPHourlyUSD          = 0.005
    EKSClusterHourlyUSD   = 0.10
    Route53ZoneMonthlyUSD = 0.50
)
type PriceQuery struct {
    ServiceCode string            // ex. "AmazonEC2"
    Region      string            // código da região, ex. "us-east-1"
    Filters     map[string]string // atributos TERM_MATCH
}
type PriceLookup interface {
    Price(ctx context.Context, q PriceQuery) (float64, error) // USD por unidade
}
var ErrNoPrice = errors.New("no price found")

// Preenche r.Cost com source "estimate", ou "none" com a nota do motivo. Nunca devolve erro.
func Estimate(ctx context.Context, r *model.Resource, m usage.Metrics, days int, p PriceLookup)
func NewPricingLookup(api pricingAPI) PriceLookup // com cache em memória, seguro para uso concorrente
func NewPricing(cfg aws.Config) pricingAPI        // cliente sempre em us-east-1
```

- [x] **Step 1: Escrever os testes que falham**

`estimate_test.go`, com um `fakePrices` que responde por `ServiceCode` e grava as consultas:

```go
func TestEstimateTable(t *testing.T)
// uma linha por tipo do contrato, com os valores e notas exatos de lá. Exemplos fixados:
// ebs gp3 1000 GB a 0.08 → 80.00, "storage only; provisioned IOPS and throughput excluded"
// ec2 rodando a 0.084/h → 61.32; ec2 parada → 0.00, "stopped; attached volumes billed separately"
// eip → 3.65; eks → 73.00, "control plane only"; route53 → 0.50, "hosted zone fee only"
// rds multi_az consulta com deploymentOption "Multi-AZ"; membro de cluster não soma armazenamento
// lambda: 3.000.000 invocações e 600.000.000 ms com 512 MB em 30 dias, a 0.0000002/req e 0.0000166667/GB-s → 5.60
// lambda com days 15 dobra o valor; dynamodb PAY_PER_REQUEST e sqs → none, "no estimate for usage-based pricing"
// aurora cluster → none, "instances are billed separately; cluster storage not estimated"
// s3 sem size_bytes → none
func TestEstimateNoEstimator(t *testing.T)  // aws.iam.role → Source none, MonthlyUSD nil, Note "not priced"
func TestEstimatePriceMissing(t *testing.T) // ErrNoPrice → none, Note "no list price found for <descrição>"
func TestEstimateLookupError(t *testing.T)  // outro erro → none, Note "pricing unavailable: <erro>"
func TestEstimateRounds(t *testing.T)       // MonthlyUSD arredondado a 2 casas
```

`pricing_test.go`, com fake de `GetProducts` devolvendo um documento da lista de preços:

```go
func TestPricingParsesOnDemand(t *testing.T) // extrai terms.OnDemand.*.priceDimensions.*.pricePerUnit.USD
func TestPricingSendsFilters(t *testing.T)   // um filtro TERM_MATCH por entrada de Filters, mais regionCode == Region
func TestPricingCaches(t *testing.T)         // duas chamadas iguais → uma chamada à API; erros não ficam em cache
func TestPricingNoProducts(t *testing.T)     // PriceList vazio → ErrNoPrice
func TestPricingSkipsZeroPrice(t *testing.T) // dimensões com preço 0 são ignoradas; só zeros → ErrNoPrice
```

- [x] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/provider/aws/cost/...` — Expected: FAIL.

- [x] **Step 3: Implementar**

`go get` de `service/pricing`. Os filtros de cada estimador (códigos de serviço e atributos como `instanceType`, `operatingSystem`, `tenancy`, `preInstalledSw`, `capacitystatus`, `volumeApiName`, `databaseEngine`, `deploymentOption`) devem ser conferidos na referência atual da AWS Price List Query API antes de fixados; quando um filtro devolver mais de um produto, usar o de menor preço diferente de zero e manter a nota do contrato. EC2: `platform` vazio conta como Linux.

- [x] **Step 4: Verificar e commitar**

Run: `go build ./... && go vet ./... && go test ./...` — Expected: PASS.

```bash
git add -A && git commit -m "feat: estimate monthly cost from list prices"
```

---

### Task 9: Custo real

**Files:**
- Create: `internal/provider/aws/cost/actual.go`
- Test: `internal/provider/aws/cost/actual_test.go`

**Interfaces:**
- Consumes: `model` (Task 1).
- Produces:

```go
const ActualWindowDays = 14

type Actual map[string]float64 // identificador devolvido pelo Cost Explorer → USD/mês projetado
func FetchActual(ctx context.Context, api costExplorerAPI, now time.Time) (a Actual, calls int, err error)
func (a Actual) Lookup(r model.Resource) (float64, bool) // tenta ARN, depois ID
func ApplyActual(r *model.Resource, monthlyUSD float64)  // Source actual, Note "projected from last 14 days"
func NewCostExplorer(cfg aws.Config) costExplorerAPI     // cliente em us-east-1
```

- [x] **Step 1: Escrever os testes que falham**

```go
func TestFetchActualProjects(t *testing.T)
// recurso com 1.00/dia em 14 dias → 30.00; recurso que só aparece em 7 dos 14 dias com 2.00/dia → 30.00 (total/14×30)
func TestFetchActualRequest(t *testing.T)
// TimePeriod de now-14d a now, em UTC e formato de data; Granularity DAILY; métrica UnblendedCost; GroupBy dimensão RESOURCE_ID
func TestFetchActualPaginates(t *testing.T) // NextPageToken → 2 chamadas; calls == 2
func TestFetchActualError(t *testing.T)     // erro da API → err != nil, a == nil, calls conta a tentativa
func TestFetchActualSkipsNoResource(t *testing.T) // grupos com chave vazia ou "NoResourceId" são ignorados
func TestLookup(t *testing.T)               // casa por ARN; senão por ID; nenhum → false
func TestApplyActual(t *testing.T)          // arredonda a 2 casas; nota exata
```

- [x] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/provider/aws/cost/... -run Actual` — Expected: FAIL.

- [x] **Step 3: Implementar**

`go get` de `service/costexplorer`. Antes de fixar a requisição, conferir na referência atual de `GetCostAndUsageWithResources` se `Filter` é obrigatório e quais serviços aceita; se exigir filtro por serviço, fazer uma chamada por serviço coberto e somar em `calls`, ajustando `TestFetchActualRequest` de acordo. Registrar no comentário do arquivo o que a documentação diz.

- [x] **Step 4: Verificar e commitar**

Run: `go build ./... && go vet ./... && go test ./...` — Expected: PASS.

```bash
git add -A && git commit -m "feat: fetch actual per-resource cost from Cost Explorer"
```

---

### Task 10: Achados de configuração

**Files:**
- Create: `internal/provider/aws/checks/checks.go`
- Test: `internal/provider/aws/checks/checks_test.go`

**Interfaces:**
- Consumes: `model` (Task 1); chaves de `Config` do contrato; `Usage` e `Cost` já preenchidos.
- Produces:

```go
package checks

const (
    CertExpiryDays      = 30
    AccessKeyMaxAgeDays = 90
    WasteMediumUSD      = 50.0
)
var DeprecatedLambdaRuntimes map[string]bool // conferir na página de runtimes da AWS ao implementar

// Devolve os achados do recurso, ordenados por gravidade (high, medium, low) e depois por Check. Nunca nil.
func Run(r model.Resource, now time.Time) []model.Finding
```

- [x] **Step 1: Escrever os testes que falham**

Tabela com um caso positivo e um negativo por verificação da seção "Achados de configuração" da spec, conferindo `Check`, `Severity` e que `Message` e `Recommendation` não são vazios. Bordas fixadas:

```go
// sg.open_admin_port: world_open_ports ["22"], ["3389"] ou ["all"] dispara; ["443"] não
// iam.user_no_mfa: só com has_console_password verdadeiro
// iam.access_key_old: 90 dias não dispara, 91 dispara, -1 não dispara
// acm.expiring: in_use_count > 0 e not_after em 29 dias dispara; em 31 não; vencido dispara com mensagem "certificate has expired";
//               not_after "" não dispara nem dá panic
// acm.unused: in_use_count == 0
// cloudfront.http_allowed: viewer_protocol_policy "allow-all"
// cloudfront.old_tls: "TLSv1", "TLSv1_2016", "TLSv1.1_2016", "SSLv3" disparam; "TLSv1.2_2021" não; "" não
// eks.public_endpoint: endpoint_public verdadeiro e public_access_cidrs contém "0.0.0.0/0"
// rds.no_backups: backup_retention_days == 0 e cluster_id == "" (membros de Aurora ficam de fora)
// usage.orphan / usage.idle: custo 50.00 → medium; 49.99 → low; custo nil → low;
//   Message inclui Usage.Reason e, se houver custo, "$80.00/mo"; in_use, unknown e not_evaluated não geram achado
func TestRunOrder(t *testing.T)     // volume gp2 órfão sem criptografia de 80/mês → [ebs.unencrypted, usage.orphan, ebs.gp2]
func TestRunNeverNil(t *testing.T)  // recurso sem achados → fatia vazia, não nil
func TestWasteRecommendationPerType(t *testing.T)
// todo tipo com regra de uso tem recomendação própria para idle e/ou orphan; tipo sem texto próprio cai em
// "Review whether this resource is still needed"
```

- [x] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/provider/aws/checks/...` — Expected: FAIL.

- [x] **Step 3: Implementar**

Mensagens e recomendações em inglês, uma frase cada, no imperativo e sem prometer economia exata. Para `rds.public` usar os textos do exemplo da spec. Recomendações de desperdício dizem o que fazer com segurança (ex. volume órfão: "Create a snapshot if the data may be needed, then delete the volume").

- [x] **Step 4: Verificar e commitar**

Run: `go build ./... && go vet ./... && go test ./...` — Expected: PASS.

```bash
git add -A && git commit -m "feat: add configuration findings and improvement suggestions"
```

---

### Task 11: Orquestração

**Files:**
- Create: `internal/provider/provider.go`, `internal/provider/aws/scan.go`, `internal/provider/aws/provider.go`
- Test: `internal/provider/aws/scan_test.go`

**Interfaces:**
- Consumes: `collect.Collector`, `collect.Scope`, `collect.All` (Tasks 5–7); `usage.QueriesFor`, `usage.Apply`, `usage.Request`, `usage.Result`, `usage.Fetch`, `usage.NewCloudWatch` (Tasks 3–4); `cost.Estimate`, `cost.PriceLookup`, `cost.NewPricingLookup`, `cost.FetchActual`, `cost.Actual`, `cost.ApplyActual` (Tasks 8–9); `checks.Run` (Task 10); `model.NewReport` (Task 1).
- Produces:

```go
package provider

type ScanOptions struct {
    Days        int
    Regions     []string // vazio = todas as habilitadas
    Types       []string // valores de Short; vazio = todos
    CostMode    string   // "auto", "estimate", "none"
    Concurrency int
    Profile     string
    Now         time.Time
    Progress    io.Writer // stderr; nunca nil
}
type Provider interface {
    Name() string
    Scan(ctx context.Context, o ScanOptions) (model.Report, error)
}
```

```go
package aws

type Deps struct {
    AccountID  func(ctx context.Context) (string, error)
    Regions    func(ctx context.Context) ([]string, error)
    Collectors []collect.Collector
    Collect    func(ctx context.Context, c collect.Collector, s collect.Scope) ([]model.Resource, error)
    Metrics    func(ctx context.Context, region string, reqs []usage.Request, start, end time.Time) map[string]usage.Result
    Actual     func(ctx context.Context, now time.Time) (cost.Actual, int, error)
    Prices     cost.PriceLookup
}
func Run(ctx context.Context, d Deps, o provider.ScanOptions) (model.Report, error)
func New() provider.Provider
```

- [x] **Step 1: Escrever os testes que falham**

Com `Deps` montado de fakes:

```go
func TestRunHappyPath(t *testing.T)
// 2 regiões, 1 coletor regional e 1 global: o regional roda 2 vezes, o global 1 vez com Scope.Region "global";
// Report.Regions ordenado; todo recurso sai com Usage, Cost e Findings preenchidos
func TestRunCollectorFailureBecomesWarning(t *testing.T)
// coletor que falha em eu-west-3 → Warning{Region:"eu-west-3", Type:<tipo>, Error:<texto>}; recursos das outras
// combinações presentes; err == nil; a linha de aviso aparece em Progress
func TestRunAccountError(t *testing.T)   // AccountID falha → err != nil, nenhum coletor chamado
func TestRunRegionsOverride(t *testing.T) // o.Regions preenchido → d.Regions não é chamado
func TestRunTypeFilter(t *testing.T)     // o.Types ["ebs"] → só coletores com Short "ebs" rodam
func TestRunMetricsRegion(t *testing.T)
// consultas agrupadas por região efetiva: Query.Region quando preenchido, senão a do recurso;
// recurso global com Query.Region vazio → "us-east-1"; start == Now - Days, end == Now
func TestRunMetricsErrorGivesUnknown(t *testing.T) // Result.Err para um lambda → veredito unknown, sem aviso de coletor
func TestRunCostAuto(t *testing.T)
// recurso presente em Actual → source actual; ausente → estimate; Progress menciona quantas chamadas ao Cost Explorer
func TestRunCostAutoFallback(t *testing.T)
// Actual devolve erro → todos estimados; exatamente um Warning com Type "cost" e Region "global"; err == nil
func TestRunCostEstimate(t *testing.T)   // modo estimate → d.Actual não é chamado
func TestRunCostNone(t *testing.T)       // modo none → Source none, Note "cost not requested"; nem Actual nem Prices chamados
func TestRunConcurrencyBound(t *testing.T) // Concurrency 2 com 6 regiões → no máximo 2 regiões em coleta ao mesmo tempo
func TestRunCancel(t *testing.T)
// contexto cancelado durante a coleta → Run retorna ctx.Err() em menos de 1 s, mesmo com coletores bloqueados em ctx.Done()
func TestRunDeterministic(t *testing.T)  // duas execuções com os mesmos fakes produzem Reports iguais
```

- [x] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/provider/aws/... -run Run` — Expected: FAIL.

- [x] **Step 3: Implementar `Run` e `New`**

Ordem: conta → regiões → coleta (semáforo de `Concurrency` regiões; coletores globais uma vez) → métricas por região efetiva → `usage.Apply` → custo (`none`; ou `Actual` + `Estimate` para o que faltar) → `checks.Run` → `model.NewReport`. Uma linha de progresso por região concluída em `o.Progress`.

`New()` monta `Deps` reais na hora do `Scan`: `config.LoadDefaultConfig` com `o.Profile`; `AccountID` via `sts:GetCallerIdentity`; `Regions` via `ec2:DescribeRegions` (só as habilitadas, sem `AllRegions`); `Collect` chama `c.Collect` com a configuração copiada para a região (`us-east-1` para globais); `Metrics` usa `usage.Fetch` com cliente da região. `go get` de `service/sts`.

- [x] **Step 4: Verificar e commitar**

Run: `go build ./... && go vet ./... && go test -race ./...` — Expected: PASS.

```bash
git add -A && git commit -m "feat: orchestrate scan across regions with partial-failure warnings"
```

---

### Task 12: Comando `scan`

**Files:**
- Create: `cmd/scan.go`
- Modify: `cmd/root.go`, `main.go`
- Test: `cmd/scan_test.go`

**Interfaces:**
- Consumes: `provider.Provider`, `provider.ScanOptions` (Task 11); `aws.New` (Task 11); `collect.Shorts` (Task 5); `output.Write*` (Task 2).
- Produces: `func Execute() error`; variável de pacote `newProvider = func() provider.Provider` trocável nos testes.

Flags e padrões: `--days 30`, `--regions ""`, `--type ""`, `--output table`, `--out-file ""`, `--cost auto`, `--profile ""`, `--concurrency 8`.

- [x] **Step 1: Escrever os testes que falham**

Com um provider falso que grava as opções e devolve um relatório fixo:

```go
func TestScanDefaults(t *testing.T)
// sem flags: Days 30, CostMode "auto", Concurrency 8, Regions e Types vazios; stdout contém "Account "
func TestScanParsesLists(t *testing.T)    // --regions us-east-1,sa-east-1 --type ec2,ebs → fatias com 2 itens, espaços aparados
func TestScanOutputJSON(t *testing.T)     // --output json → stdout é JSON válido com schema_version "1"; nada além do JSON
func TestScanOutputCSV(t *testing.T)      // primeira linha começa com "provider,account_id,"
func TestScanOutFile(t *testing.T)        // --out-file em diretório temporário → arquivo gravado, stdout vazio
func TestScanOutFileError(t *testing.T)   // caminho em diretório inexistente → erro, provider não chamado
func TestScanInvalidArgs(t *testing.T)
// casos: --days 0; --days -3; --output xml; --cost real; --concurrency 0; --type foo
// cada um: erro não nulo, provider não chamado, mensagem cita a flag e os valores aceitos
// ("--type foo" lista os valores de collect.Shorts())
func TestScanProviderError(t *testing.T)  // erro do provider propagado; nada em stdout
func TestScanWarningsToStderr(t *testing.T) // relatório com avisos → cada um em stderr; stdout inalterado
```

- [x] **Step 2: Rodar e ver falhar**

Run: `go test ./cmd/...` — Expected: FAIL.

- [x] **Step 3: Implementar**

Validar tudo e abrir o arquivo de saída antes de chamar o provider. `signal.NotifyContext` com `os.Interrupt` para cancelar a varredura. `main.go`: erro em stderr e `os.Exit(1)`; varredura concluída com avisos sai com 0.

- [x] **Step 4: Verificar de ponta a ponta**

Run: `go build -o /tmp/lookr . && /tmp/lookr scan --type foo; echo "exit=$?"` — Expected: mensagem de erro com a lista de tipos e `exit=1`.
Run: `/tmp/lookr scan --help` — Expected: as oito flags com seus padrões.
Run: `go build ./... && go vet ./... && go test -race ./...` — Expected: PASS.

Se houver credenciais AWS disponíveis, rodar também `/tmp/lookr scan --regions us-east-1 --cost estimate --output json | head -40` e registrar o resultado no relato da tarefa; sem credenciais, dizer que essa verificação não foi feita.

- [x] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add lookr scan command"
```

---

### Task 13: README e release

**Files:**
- Modify: `README.md`, `.goreleaser.yaml`, `.github/workflows/release.yaml`, `.gitignore` (acrescentar `.DS_Store`)

**Interfaces:**
- Consumes: flags da Task 12; formato de saída das Tasks 1 e 2; lista de verificações da Task 10.

- [x] **Step 1: Reescrever o README**

Em inglês, cobrindo: o que o `lookr scan` faz; instalação (`go install` e `go build -o lookr .`); uso com exemplos das três saídas; tabela das flags; tipos suportados com o `short` de cada um; significado dos cinco vereditos e das três origens de custo; lista das verificações com gravidade; limites conhecidos (estimativa ignora Reserved Instances, Savings Plans, Spot e tráfego; custo real cobre 14 dias e precisa ser ativado na conta pagadora; chamadas ao Cost Explorer custam US$ 0,01 cada e `--cost estimate` as evita; S3 fica `unknown` por falta de métrica de acesso); política IAM mínima somente leitura em JSON, com as ações que os coletores de fato chamam, mais `cloudwatch:GetMetricData`, `pricing:GetProducts`, `ce:GetCostAndUsageWithResources` e `sts:GetCallerIdentity`; aviso de que os comandos antigos (`lookr ec2` etc.) foram substituídos por `lookr scan --type ec2`.

- [x] **Step 2: Conferir a política contra o código**

Run: `grep -rhoE '\.(Describe|List|Get)[A-Za-z]+\(' internal/provider/aws --include='*.go' --exclude='*_test.go' | sort -u`
Expected: toda operação listada tem ação correspondente na política do README, e a busca `grep -rnE '\.(Create|Delete|Put|Update|Modify|Terminate|Stop|Start)[A-Z][A-Za-z]+\(' internal/provider/aws --include='*.go' --exclude='*_test.go'` não devolve nada.

- [x] **Step 3: Corrigir a configuração de release**

`.goreleaser.yaml`: `CG0_ENABLED=0` → `CGO_ENABLED=0`; acrescentar `version: 2` no topo e ajustar as chaves que a versão 2 renomeou. `release.yaml`: `actions/checkout@v4`, `actions/setup-go@v5` com `go-version-file: go.mod`, `goreleaser/goreleaser-action@v6`, `args: release --clean`.

Run (se o `goreleaser` estiver instalado): `goreleaser check` — Expected: sem erros. Sem o binário, dizer que a checagem não foi feita.

- [x] **Step 4: Verificação final e commit**

Run: `go build ./... && go vet ./... && go test -race ./...` — Expected: PASS.

```bash
git add -A && git commit -m "docs: rewrite README for scan and fix release config"
```
