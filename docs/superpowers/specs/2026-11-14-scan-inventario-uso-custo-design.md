# lookr scan: inventário com uso e custo por recurso

Data: 2026-11-14
Status: aguardando aprovação

## Intenção

Hoje o `lookr` lista um serviço AWS por vez e imprime uma tabela. O que se quer é outra coisa: varrer uma conta inteira e, para cada recurso, saber **como está configurado, quanto custa e se está de fato sendo usado**. O objetivo final é enxergar o que está parado gerando conta.

O resultado é lido por uma pessoa no terminal, mas também precisa sair em JSON e CSV, porque pode ser consumido por outra ferramenta ou por uma IA.

## Objetivos

- Um comando único, `lookr scan`, que varre todos os tipos de recurso suportados em todas as regiões habilitadas da conta.
- Para cada recurso: configuração, veredito de uso com o motivo, e custo mensal com a origem do número.
- Custo real quando a conta permitir; estimativa no resto; cada linha diz qual dos dois é.
- Saída em tabela (terminal), JSON e CSV, com formato estável e versionado.
- Estrutura que aceite outro cloud provider depois sem mexer no que já existe.
- Somente leitura.

## Fora de escopo nesta versão

- Outros providers além de AWS.
- Várias contas numa execução (AWS Organizations). Uma execução, uma conta.
- Relatório em HTML ou PDF.
- Histórico e comparação entre varreduras.
- Qualquer ação de correção (parar, apagar, redimensionar).
- Auditoria completa de conformidade (CIS, PCI etc.). Entra só o conjunto pequeno de verificações listado em "Achados de configuração".
- Limiares de ociosidade configuráveis por arquivo. Só `--days` é ajustável.

## O que seria um fracasso mesmo funcionando

- **Dizer "ocioso" para algo que está em uso.** Alguém pode apagar o recurso com base nisso. Na dúvida, o veredito é `unknown`, nunca `idle`.
- **Custo sem origem.** Um número estimado apresentado como se fosse o cobrado.
- **Varredura que morre inteira** porque uma região ou um serviço negou acesso.
- **Qualquer chamada de escrita** na conta.
- **Cobrança inesperada.** As chamadas pagas (Cost Explorer, CloudWatch) precisam ser poucas, avisadas e desligáveis.

## Caminhos considerados

**A. Reaproveitar o código atual.** Manter o SDK v1 e os 14 arquivos em `cmd/`, acrescentando uso e custo em cada um. Mais rápido para ter algo rodando, mas carrega SDK sem suporte desde 2025, varredura sequencial e a mesma lógica repetida 14 vezes.

**B. Reescrever o núcleo com coletores (escolhido).** SDK v2 e três etapas separadas: coleta, uso, custo. Mais trabalho inicial, quase nada do código atual sobrevive, mas o trabalho por tipo de recurso fica organizado e outro provider entra como um novo conjunto de coletores.

**C. Descoberta genérica pela AWS.** Resource Explorer ou AWS Config para listar tudo sem coletor por serviço. Cobre mais tipos com menos código, mas depende de serviços ativados na conta (Config é pago) e as regras de uso e custo continuam sendo por tipo.

Escolhido o **B** pelo usuário. O valor do produto está no veredito e no custo por recurso, que são trabalho por tipo em qualquer caminho.

## Como funciona

### Fluxo

1. Resolve credenciais (cadeia padrão da AWS, ou `--profile`) e identifica a conta via STS. Se falhar, encerra com erro.
2. Descobre as regiões habilitadas na conta (`ec2:DescribeRegions`), ou usa `--regions`.
3. **Coleta**: para cada região, em paralelo, roda um coletor por tipo de recurso. Serviços globais (CloudFront, IAM, Route53, S3) rodam uma vez só, com região `global` (S3 guarda a região real do bucket na configuração).
4. **Uso**: busca métricas do CloudWatch para a janela (`--days`, padrão 30) e aplica a regra do tipo para chegar ao veredito.
5. **Custo**: tenta o custo real por recurso; o que não vier, estima.
6. **Saída**: tabela, JSON ou CSV.

Erro em uma combinação região + tipo vira um aviso (em stderr e no campo `warnings` da saída) e a varredura continua.

### Formato de um recurso

Todos os coletores devolvem o mesmo formato. Nomes de campo e valores em inglês, por serem lidos por máquina.

```json
{
  "provider": "aws",
  "account_id": "123456789012",
  "region": "us-east-1",
  "type": "aws.ebs.volume",
  "id": "vol-0a1b2c3d",
  "arn": "arn:aws:ec2:us-east-1:123456789012:volume/vol-0a1b2c3d",
  "name": "dados-antigos",
  "tags": {"env": "homolog"},
  "created_at": "2025-03-02T14:11:00Z",
  "config": {"size_gb": 1000, "volume_type": "gp3", "iops": 3000, "encrypted": true, "state": "available"},
  "usage": {
    "verdict": "orphan",
    "reason": "not attached to any instance",
    "window_days": 30,
    "metrics": {}
  },
  "cost": {
    "monthly_usd": 80.0,
    "source": "estimate",
    "note": "on-demand list price"
  }
}
```

- `usage.verdict`: `in_use`, `idle`, `orphan`, `unknown`, `not_evaluated`.
  - `idle`: existe e funciona, mas sem atividade na janela.
  - `orphan`: sobrou sem aquilo a que servia (volume desanexado, IP não associado).
  - `unknown`: deveria ter sido avaliado, mas faltou dado (sem métrica, sem permissão, recurso novo demais).
  - `not_evaluated`: tipo que só entra como inventário.
- `usage.metrics`: os números que sustentam o veredito, para quem lê poder conferir.
- `cost.source`: `actual`, `estimate` ou `none`. Com `none`, `monthly_usd` é `null` e `note` diz por quê.
- `config`: campos específicos do tipo. É onde ficam os dados de configuração que não cabem na tabela.

### Documento de saída (JSON)

```json
{
  "schema_version": "1",
  "generated_at": "2026-11-14T12:00:00Z",
  "provider": "aws",
  "account_id": "123456789012",
  "window_days": 30,
  "regions": ["us-east-1", "sa-east-1"],
  "summary": {
    "resources": 214,
    "monthly_usd": 3482.10,
    "waste_resources": 37,
    "waste_monthly_usd": 912.40,
    "by_verdict": {"in_use": 150, "idle": 25, "orphan": 12, "unknown": 9, "not_evaluated": 18}
  },
  "resources": [],
  "warnings": [{"region": "eu-west-3", "type": "aws.rds.instance", "error": "AccessDenied"}]
}
```

"Waste" é a soma de `idle` e `orphan`.

### CSV

Uma linha por recurso, colunas fixas: `provider, account_id, region, type, id, arn, name, created_at, verdict, reason, monthly_usd, cost_source, cost_note, tags, config, metrics`. As três últimas são JSON em texto dentro da célula. O resumo e os avisos não entram no CSV; avisos vão para stderr.

### Tabela no terminal

Cabeçalho com o resumo, depois os recursos ordenados por veredito (`orphan`, `idle`, `unknown`, `in_use`, `not_evaluated`) e, dentro de cada um, por custo decrescente.

```
Account 123456789012 · 11 regions · 214 resources · $3,482/mo
Idle or orphan: 37 resources · $912/mo (26%)

TYPE  RESOURCE         REGION     VERDICT  REASON                     COST/MO
ebs   vol-0a1b…        us-east-1  orphan   not attached               $80.00 (est.)
ec2   i-04f2… (api-2)  sa-east-1  idle     avg CPU 0.8% over 30d      $61.32 (actual)
elb   lb-interno       us-east-1  idle     0 requests over 30d        $18.40 (est.)
rds   db-homolog       us-east-1  in_use   max 14 connections         $142.10 (actual)
```

Progresso e avisos vão para stderr, dados para stdout, de modo que `lookr scan --output json > inventario.json` gera um arquivo limpo.

### Linha de comando

```
lookr scan [flags]

--days N            janela de análise de uso, padrão 30
--regions a,b       restringe as regiões; padrão: todas as habilitadas na conta
--type a,b          restringe os tipos (ex.: ec2,ebs,rds); padrão: todos
--output FORMAT     table | json | csv; padrão table
--out-file PATH     grava em arquivo em vez de stdout
--cost MODE         auto | estimate | none; padrão auto
--profile NAME      perfil de credenciais da AWS
--concurrency N     regiões em paralelo, padrão 8
```

Os 14 comandos atuais (`lookr ec2`, `lookr rds` etc.) deixam de existir. O equivalente é `lookr scan --type ec2`.

### Tipos de recurso e regras de uso

Janela padrão de 30 dias. Recurso criado há menos de 7 dias recebe `unknown` ("too new to evaluate"), qualquer que seja a métrica. Métrica ausente ou sem permissão também dá `unknown`.

| Tipo | Veredito |
|---|---|
| EC2 instance | parada → `idle`; rodando com CPU média < 2% e máxima < 10% e tráfego de rede desprezível → `idle` |
| EBS volume | estado `available` → `orphan`; anexado a instância parada → `idle` |
| EBS snapshot | volume de origem não existe mais, snapshot não é usado por AMI e é mais antigo que a janela → `orphan`; senão `not_evaluated` |
| Elastic IP | sem associação → `orphan` |
| NAT Gateway | zero bytes trafegados na janela → `idle` |
| Load balancer (ALB, NLB, clássico) | sem alvos registrados → `orphan`; zero requisições ou fluxos → `idle` |
| RDS instance / Aurora cluster | parado → `idle`; zero conexões na janela → `idle` |
| ElastiCache | zero comandos na janela → `idle` |
| EKS cluster | sem node groups, nós nem perfis Fargate → `idle`; senão `in_use` |
| Lambda | zero invocações → `idle` |
| DynamoDB table | zero leitura e escrita consumidas → `idle` |
| SQS queue | zero mensagens enviadas e recebidas → `idle` |
| CloudFront distribution | zero requisições → `idle`; desabilitada → `idle` |
| S3 bucket | vazio → `idle`; senão `unknown` (a AWS não fornece métrica de acesso por padrão) |
| IAM (users, groups, roles), ACM, Route53 | `not_evaluated`, só inventário |

Os limiares numéricos ficam todos num lugar só do código, com nome, para serem fáceis de revisar.

### Custo

**Real (`actual`)**: Cost Explorer com granularidade por recurso (`GetCostAndUsageWithResources`). Só existe se ativado na conta pagadora e cobre apenas os últimos 14 dias. O valor mensal é a média diária do período disponível × 30, e `note` diz isso ("projected from last 14 days"). A AWS cobra US$ 0,01 por chamada; a varredura agrupa as chamadas por serviço e informa em stderr quantas fez.

**Estimado (`estimate`)**: preço de tabela on-demand da AWS Pricing API × configuração do recurso, com cache em memória durante a execução. Não considera Reserved Instances, Savings Plans, Spot nem tráfego de dados. Cobre bem o que é cobrado por hora ou por capacidade provisionada (EC2, EBS, snapshots, Elastic IP, NAT Gateway, load balancers, RDS, ElastiCache, EKS, DynamoDB provisionado, armazenamento do S3). Para o que é cobrado por uso (Lambda, SQS, DynamoDB sob demanda, CloudFront), estima a partir das métricas de uso da janela quando a conta é simples; quando não é, fica `none` com nota "no estimate for usage-based pricing".

**Modos**:
- `auto` (padrão): tenta o real; se o Cost Explorer não estiver disponível ou não trouxer o recurso, estima. Um aviso único em stderr informa que caiu para estimativa e por quê.
- `estimate`: não chama o Cost Explorer.
- `none`: não calcula custo.

IAM, ACM e Route53 ficam com `none` (exceto zonas hospedadas do Route53, que têm preço fixo mensal e podem ser estimadas).

### Achados de configuração e sugestões

Acrescentado em 2026-11-14, a pedido do usuário, depois da primeira aprovação.

Além do veredito de uso, cada recurso carrega uma lista `findings`: erros de configuração encontrados e sugestões de melhoria. Cada achado tem:

```json
{
  "check": "rds.public",
  "severity": "high",
  "message": "database is publicly accessible",
  "recommendation": "Disable public accessibility or restrict the security group to known addresses"
}
```

- `severity`: `high`, `medium` ou `low`.
- `findings` é sempre uma lista, vazia quando não há nada.
- O resumo do relatório ganha `findings_by_severity`.
- Na tabela, uma coluna `ISSUES` traz a contagem por recurso e um bloco `FINDINGS` abaixo lista os achados, dos mais graves para os menos.
- No CSV, uma coluna `findings` com o JSON em texto.

As verificações são funções puras sobre a configuração já coletada. Conjunto inicial:

| Verificação | Gravidade | Condição |
|---|---|---|
| `sg.open_admin_port` | high | security group libera 22 ou 3389 para 0.0.0.0/0 ou ::/0 |
| `rds.public` | high | banco com acesso público habilitado |
| `s3.public_access_not_blocked` | high | bucket sem bloqueio de acesso público completo |
| `iam.user_no_mfa` | high | usuário com senha de console e sem MFA |
| `acm.expiring` | high | certificado em uso que vence em menos de 30 dias, ou já vencido |
| `ebs.unencrypted` | medium | volume sem criptografia |
| `rds.unencrypted` | medium | armazenamento sem criptografia |
| `rds.no_backups` | medium | retenção de backup igual a zero |
| `ec2.imdsv1` | medium | instância aceita IMDSv1 |
| `iam.access_key_old` | medium | chave de acesso ativa com mais de 90 dias |
| `cloudfront.http_allowed` | medium | distribuição aceita HTTP sem redirecionar |
| `cloudfront.old_tls` | medium | versão mínima de TLS abaixo de 1.2 |
| `eks.public_endpoint` | medium | endpoint público aberto para 0.0.0.0/0 |
| `lambda.deprecated_runtime` | medium | runtime fora de suporte |
| `ebs.gp2` | low | volume gp2 (gp3 é mais barato) |
| `sqs.unencrypted` | low | fila sem criptografia |
| `acm.unused` | low | certificado não associado a nenhum recurso |
| `usage.orphan` / `usage.idle` | medium se custa US$ 50/mês ou mais, senão low | veredito de desperdício, com sugestão do que fazer por tipo |

Para dar suporte à primeira linha, security groups entram como tipo de recurso (só inventário, sem veredito de uso).

O `lookr` continua só lendo: as sugestões são texto, nenhuma é aplicada.

### Organização do código

```
main.go
cmd/                      root e scan (cobra)
internal/model/           Resource, Usage, Cost, Report
internal/provider/        interface Provider (Name, Scan)
internal/provider/aws/    sessão, regiões, orquestração paralela
  collect/                um arquivo por tipo de recurso
  usage/                  consulta ao CloudWatch e regras por tipo
  cost/                   Cost Explorer e Pricing API
  checks/                 achados de configuração e sugestões
internal/output/          table, json, csv
```

- AWS SDK for Go v2. O SDK v1 e o pacote `deps/` saem.
- Cada tipo registra três peças: um coletor (obrigatório), uma regra de uso e um estimador de custo (opcionais). Acrescentar um tipo é acrescentar um arquivo em cada pasta, sem tocar na orquestração.
- As regras de uso são funções puras: recebem o recurso e as métricas, devolvem veredito e motivo.
- Métricas buscadas com `GetMetricData` em lotes, para reduzir chamadas.
- Todas as listagens paginam.

### Permissões

O README passa a trazer a política IAM mínima, somente leitura: as ações `Describe*`/`List*`/`Get*` dos serviços cobertos, `cloudwatch:GetMetricData`, `pricing:GetProducts`, `ce:GetCostAndUsageWithResources` e `sts:GetCallerIdentity`.

### Testes

- Regras de uso: testes em tabela, cobrindo cada veredito e os casos de dado ausente e recurso novo.
- Coletores: clientes da AWS atrás de interfaces pequenas, testados com implementações falsas. Nenhum teste chama a AWS de verdade.
- Estimadores de custo: preços fixos injetados.
- Saídas: arquivos de referência para JSON, CSV e tabela.
- Orquestração: um coletor que falha não impede os outros, e o erro aparece em `warnings`.

### Arrumações que entram junto

- `.goreleaser.yaml`: `CG0_ENABLED` → `CGO_ENABLED`.
- `release.yaml`: `--rm-dist` → `--clean`, e versões atuais das actions.
- README reescrito para o novo comando, com o formato de saída documentado.

## Decisões tomadas pelo autor do design (não discutidas com o usuário)

- Saída do CLI e campos do JSON em inglês, seguindo o README e o código atuais.
- Regiões descobertas na conta em vez da lista fixa de `deps/regions.go`.
- `--cost auto` como padrão, mesmo fazendo chamadas pagas ao Cost Explorer.
- Recurso com menos de 7 dias recebe `unknown`.
- Os limiares da tabela de regras (CPU 2%/10% etc.).
- Quinto veredito `not_evaluated`, separado de `unknown`.
- Código de saída 0 quando a varredura termina com avisos parciais; 1 só em falha de credenciais ou de argumentos.
- Zonas do Route53 estimadas pelo preço fixo.
- A lista inicial de verificações de configuração, suas gravidades e os limites (30 dias para certificado, 90 dias para chave de acesso, US$ 50/mês para elevar a gravidade de desperdício).
- Métricas de contador sem nenhum ponto no CloudWatch contam como zero (a AWS não publica pontos quando não há atividade); só falha na consulta ou ausência de métricas de nível (CPU, conexões) dá `unknown`.
- Clusters EKS também contam instâncias EC2 com a tag do cluster, para não chamar de ocioso um cluster com nós autogerenciados.

## Deixado para quem implementar

- Nomes exatos de pacotes, interfaces e funções.
- Filtros exatos da Pricing API por tipo, e quais tipos cobrados por uso recebem estimativa.
- Conferir na documentação atual da AWS o comportamento de `GetCostAndUsageWithResources` (serviços cobertos, formato do identificador de recurso, filtros obrigatórios) e como casar o identificador retornado com o recurso coletado.
- Definição precisa de "tráfego de rede desprezível" para EC2 e a métrica de comandos do ElastiCache por engine.
- Versão do Go e das dependências.
- Ordem de implementação dos tipos.
