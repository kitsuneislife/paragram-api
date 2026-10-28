# Paragram

API de rede social de fotos e vídeos (estilo Instagram), para ser consumida por qualquer app.

Arquitetura, convenções e roadmap: [docs/](docs/README.md). Contrato HTTP: [api/openapi.yaml](api/openapi.yaml).

## Requisitos

- Go (versão em `go.mod`)
- Docker (Postgres, Redis e MinIO locais, e testes de integração)

## Rodar localmente

```bash
docker compose up -d
cp .env.example .env
```

Criar o bucket de mídia com leitura pública (uma vez):

```bash
docker compose exec minio sh -c "mc alias set local http://localhost:9000 paragram paragram-dev-secret && mc mb -p local/media && mc anonymous set download local/media"
```

Carregar o `.env` no shell:

```bash
set -a; . ./.env; set +a
```

```powershell
Get-Content .env | ForEach-Object { if ($_ -match '^([^#=]+)=(.*)$') { Set-Item "env:$($matches[1])" $matches[2] } }
```

Aplicar migrations e subir a API:

```bash
go run ./cmd/paragram migrate
go run ./cmd/paragram api
go run ./cmd/paragram worker   # outro terminal: processa mídia
```

Checar: `curl localhost:8080/readyz`.

## Testes

```bash
go test ./...
```

Testes que precisam de Postgres usam testcontainers e são pulados quando o Docker não está rodando. No CI eles sempre rodam.

Sem Docker, aponte para um Postgres próprio (cada teste cria e apaga seu banco):

```bash
PARAGRAM_TEST_DATABASE_URL="postgres://postgres@localhost:5432/postgres?sslmode=disable" go test ./...
```

Redis e S3 rodam em memória nos testes (miniredis, gofakes3).

## Estrutura

```
cmd/paragram/         entrypoint: subcomandos api, worker e migrate
internal/api/         HTTP: rotas, middleware, formato de resposta e erro
internal/auth/        hash de senha e tokens
internal/jobs/        jobs em background (River): processamento de mídia
internal/media/       redimensiona imagens
internal/storage/     bucket S3-compatível
internal/config/      configuração via variáveis de ambiente
internal/db/          conexão Postgres e migrations
  migrations/         SQL do goose, embutido no binário
  queries/            SQL de onde o sqlc gera código (a partir da fase 1)
api/openapi.yaml      contrato HTTP, fonte da verdade
docs/                 arquitetura, convenções, regras do banco, roadmap
compose.yaml          dependências locais
```

## Variáveis de ambiente

| Variável | Obrigatória | Padrão | Descrição |
|---|---|---|---|
| `DATABASE_URL` | sim | | Connection string do Postgres |
| `REDIS_URL` | sim | | Connection string do Redis |
| `PARAGRAM_JWT_SECRET` | sim | | Chave HMAC dos access tokens, mínimo 32 bytes |
| `PARAGRAM_S3_ENDPOINT` | sim | | URL do storage S3-compatível (MinIO, R2) |
| `PARAGRAM_S3_ACCESS_KEY`, `PARAGRAM_S3_SECRET_KEY` | sim | | Credenciais do storage |
| `PARAGRAM_S3_BUCKET` | sim | | Bucket de mídia |
| `PARAGRAM_S3_PUBLIC_URL` | sim | | URL base pública dos objetos do bucket |
| `PARAGRAM_ADDR` | não | `:8080` | Endereço HTTP |
| `PARAGRAM_CORS_ORIGINS` | não | vazio | Origens de navegador permitidas, separadas por vírgula |

## Commits

[Conventional Commits](https://www.conventionalcommits.org): `type(scope): resumo no imperativo`, minúsculo, até 72 caracteres. Um commit = uma mudança lógica, que compila e passa nos testes.
