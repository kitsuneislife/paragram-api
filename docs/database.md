# Banco de dados: regras de integridade

O PostgreSQL é a fonte da verdade. Regra que pode ser garantida pelo banco **é** garantida pelo banco, além da validação no código.

## Convenções

- Tabelas no plural, `snake_case` (`users`, `post_media`, `follows`).
- Chave primária `id uuid` (UUIDv7, gerado pela aplicação).
- Toda tabela tem `created_at timestamptz NOT NULL DEFAULT now()`. Tabelas editáveis têm `updated_at`.
- Datas sempre `timestamptz` (UTC).
- Migrations com goose, em SQL, nunca editadas depois de aplicadas em produção.

## Chaves estrangeiras

- Toda relação tem FK.
- `ON DELETE` escolhido de propósito em cada FK:
  - `CASCADE` para dados que não fazem sentido sem o pai (likes de um post excluído).
  - `RESTRICT` quando a exclusão deve ser explícita.
  - `SET NULL` raro, só quando o filho sobrevive sem o pai.

## Unicidade

| Regra | Como |
|---|---|
| Username único, sem diferenciar caixa | `UNIQUE` em `lower(username)` (ou tipo `citext`) |
| Email único | `UNIQUE` em `lower(email)` |
| Seguir uma vez | `PRIMARY KEY (follower_id, followee_id)` |
| Curtir uma vez | `PRIMARY KEY (user_id, post_id)` |
| Salvar uma vez | `PRIMARY KEY (user_id, post_id)` |
| Bloquear uma vez | `PRIMARY KEY (blocker_id, blocked_id)` |
| Idempotency key | `UNIQUE (user_id, key)` |

## Checagens (CHECK)

- Não seguir a si mesmo: `CHECK (follower_id <> followee_id)`.
- Não bloquear a si mesmo: `CHECK (blocker_id <> blocked_id)`.
- Carrossel com no máximo 10 mídias: `CHECK (position BETWEEN 0 AND 9)` + `UNIQUE (post_id, position)`.
- Tamanhos: bio ≤ 150, legenda ≤ 2200, username 1–30 caracteres `[a-z0-9._]`.
- Contadores nunca negativos: `CHECK (followers_count >= 0)` etc.
- Status com valores fixos (`processing`, `ready`, `failed`) como `CHECK` ou enum.

## Contadores desnormalizados

`posts_count`, `followers_count`, `following_count`, `likes_count`, `comments_count`:

- Atualizados **na mesma transação** da ação (curtir insere o like e incrementa o contador juntos).
- Job periódico recalcula a partir da fonte e corrige divergências.
- Exceção: views de vídeo ficam no Redis e são gravadas em lote (aceitamos pequena perda nesse caso).

## Exclusão

- **Soft delete** (`deleted_at timestamptz`) onde existe "desfazer" ou prazo: contas (30 dias), posts arquivados.
- **Hard delete** quando a LGPD exige (conta após o prazo, pedido de remoção) e para dados efêmeros (stories expirados, likes desfeitos).
- Queries de leitura sempre filtram `deleted_at IS NULL`. Índices parciais com `WHERE deleted_at IS NULL`.

## Transações

- Toda escrita que toca mais de uma tabela roda numa transação.
- Jobs enfileirados na mesma transação da escrita que os origina (fila em tabela do Postgres), então nunca existe post criado sem job de processamento, nem job de post que não existe.

## Índices essenciais

- Feed: `posts (author_id, created_at DESC)`.
- Seguidores: `follows (followee_id, created_at DESC)`; seguindo: `follows (follower_id, created_at DESC)`.
- Comentários: `comments (post_id, created_at)`.
- Notificações: `notifications (user_id, created_at DESC)`.
- Busca: índice GIN `pg_trgm` em `username` e `display_name`.
- Stories ativos: `stories (author_id, expires_at)`.
