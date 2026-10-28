# Convenções da API

Regras que valem para **todos** os endpoints. Endpoint novo segue estas regras; mudar uma regra significa mudar em todo lugar.

## Contrato

- OpenAPI 3.1 é a fonte da verdade. Endpoint que não está no OpenAPI não existe.
- REST + JSON (`application/json`, UTF-8).
- Campos em `snake_case`.
- WebSocket só para DM e notificações em tempo real.

## Versionamento

- Prefixo na URL: `/v1/...`.
- Mudança que quebra cliente (remover campo, mudar tipo, mudar significado) exige `/v2`.
- Adicionar campo ou endpoint não quebra e não muda a versão. Clientes devem ignorar campos desconhecidos.

## Autenticação

- Header `Authorization: Bearer <access_token>`.
- Access token expira em 15 min. Renovar com `POST /v1/auth/refresh` enviando o refresh token.
- Cada refresh gera um novo refresh token e invalida o anterior (rotação). Reuso de refresh token antigo derruba a sessão inteira (indica roubo).

## Erros

Formato único: **RFC 9457** (`application/problem+json`).

```json
{
  "type": "https://paragram.dev/errors/validation",
  "title": "Validation failed",
  "status": 422,
  "detail": "username already taken",
  "errors": [{ "field": "username", "code": "taken" }]
}
```

Status usados:

| Status | Quando |
|---|---|
| 400 | Requisição malformada (JSON inválido) |
| 401 | Sem token ou token inválido/expirado |
| 403 | Autenticado, mas sem permissão |
| 404 | Recurso não existe **ou** o usuário não pode vê-lo (não revelar existência de conta privada/bloqueada) |
| 409 | Conflito de estado |
| 422 | Validação falhou |
| 429 | Rate limit. Header `Retry-After` |
| 500 | Erro interno. Nunca expõe detalhes; o `detail` traz só o request ID |

## Paginação

Sempre por **cursor** (keyset), nunca `OFFSET`.

```
GET /v1/users/{id}/followers?limit=20&cursor=eyJ...
```

```json
{
  "data": [ ... ],
  "next_cursor": "eyJ..."
}
```

- `limit`: padrão 20, máximo 100.
- `next_cursor` ausente ou `null` = fim da lista.
- Cursor é opaco para o cliente (base64 de `created_at` + `id`).

## Idempotência

- POSTs que criam algo importante (post, comentário, mensagem, pagamento futuro) aceitam header `Idempotency-Key: <uuid>`.
- Mesma chave + mesmo usuário retorna o recurso original sem criar duplicado. Hoje a chave não expira; cliente deve gerar uma nova por ação.
- Ações naturalmente idempotentes (curtir, seguir, salvar) usam `PUT`/`DELETE` e não precisam da chave:
  - `PUT /v1/posts/{id}/like` curte (repetir não duplica).
  - `DELETE /v1/posts/{id}/like` descurte.

## IDs e datas

- IDs: **UUIDv7** (ordenáveis por tempo, não enumeráveis).
- Datas: ISO 8601 em UTC (`2026-10-28T14:30:00Z`). No banco, `timestamptz`.

## Cache HTTP

- Respostas de leitura retornam `ETag`. Cliente envia `If-None-Match` e recebe `304` se nada mudou.
- URLs de mídia são imutáveis (mudou o arquivo, muda a URL) e servidas com cache longo via CDN.

## Limites

- Rate limit por IP e por usuário. Headers `RateLimit-Limit`, `RateLimit-Remaining`, `RateLimit-Reset`.
- Corpo da requisição: máximo 1 MB (mídia não passa pela API).
- Timeout em tudo: requisição HTTP, query no banco, chamada externa.

## Operação

- `GET /healthz`: processo está vivo.
- `GET /readyz`: processo está pronto (banco e Redis acessíveis).
- Todo request recebe um `X-Request-Id` (gerado se o cliente não enviar), presente nos logs e nas respostas de erro.
- Desligamento gracioso: para de aceitar conexões, termina as em andamento, depois encerra.
- CORS configurável por ambiente.
