# Stack

Resumo: **Go + PostgreSQL + Redis + armazenamento S3-compatível.**

## Linguagem: Go (1.23+)

- `net/http` da biblioteca padrão. O roteador nativo já suporta método e parâmetros de rota (`GET /v1/users/{id}`), então não precisamos de framework.
- Binário único (~15 MB), sobe em milissegundos, consome pouca memória.
- Concorrência nativa (goroutines) para WebSocket, workers e chamadas paralelas.
- Tipagem estática e compilação rápida.
- Logs com `log/slog` (stdlib).

**Por que não Rust (axum)?** Um pouco mais rápido, mas o desenvolvimento é 2–3x mais lento. Para uma API de CRUD + feed, o gargalo é o banco, não a linguagem.

## Banco: PostgreSQL (16+)

- Fonte da verdade de todos os dados.
- Integridade via FK, UNIQUE, CHECK e transações (ver [database.md](database.md)).
- Busca com extensão `pg_trgm` + full-text search nativo. Sem Elasticsearch.
- Fila de jobs em tabela, com `SELECT ... FOR UPDATE SKIP LOCKED` (lib [River](https://riverqueue.com)). Sem RabbitMQ/Kafka no início.

### Acesso ao banco

- **pgx**: driver.
- **sqlc**: gera código Go tipado a partir de SQL puro. Erro de query aparece na compilação. Sem ORM.
- **goose**: migrations versionadas em SQL.

## Cache e tempo real: Redis

- Rate limit (token bucket).
- Cache de feed e contadores quentes (views de vídeo, com flush periódico para o Postgres).
- Presença online e indicador "digitando".
- Pub/sub para distribuir eventos de WebSocket entre instâncias da API.

O Redis nunca é a fonte da verdade. Se for apagado, nada se perde além de cache.

## Mídia: armazenamento S3-compatível

- Produção: **Cloudflare R2** (sem custo de saída de dados).
- Desenvolvimento: **MinIO** local.
- O app sobe o arquivo direto para o storage via **presigned URL**. O binário nunca passa pela API.

### Processamento

Worker separado da API:

- Imagens: Go puro (stdlib + `golang.org/x/image`) gera tamanhos em JPEG. Reencodar remove o EXIF. Sem cgo, build igual em Windows e Linux. Trocar por `libvips` se WebP/AVIF ou velocidade virarem requisito.
- Vídeos: `ffmpeg` transcodifica para HLS e gera thumbnail.

## Contrato e clientes

- **OpenAPI 3.1** descreve todos os endpoints.
- REST + JSON para tudo, WebSocket apenas para DM e notificações em tempo real.
- Clientes (Swift, Kotlin, TypeScript, Dart) gerados a partir do OpenAPI.

## Notificações push

- FCM (Android/web) e APNs (iOS).

## Observabilidade

- Logs estruturados com request ID.
- Métricas Prometheus.
- Tracing OpenTelemetry.

## Testes

- Unitários na lógica de domínio (pura, sem I/O).
- Integração contra Postgres real via testcontainers.

## Deploy

- Docker com imagem distroless.
- Uma VPS pequena (Fly.io, Hetzner) basta por muito tempo. Kubernetes só quando houver necessidade real.
- Processos: `api` (HTTP + WebSocket) e `worker` (jobs e mídia), mesmo binário com comandos diferentes.
