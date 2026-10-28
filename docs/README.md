# Paragram — documentação

API de uma rede social de fotos e vídeos (estilo Instagram), feita para ser consumida por qualquer app (mobile, web, desktop).

Objetivos, em ordem: **correção e integridade dos dados**, consistência, simplicidade, velocidade.

## Índice

| Documento | Conteúdo |
|---|---|
| [stack.md](stack.md) | Linguagem, banco, infraestrutura e por que cada escolha |
| [features.md](features.md) | Tudo que a API oferece, por domínio |
| [api-conventions.md](api-conventions.md) | Regras de contrato: versionamento, erros, paginação, auth, idempotência |
| [database.md](database.md) | Regras de integridade garantidas pelo banco |
| [roadmap.md](roadmap.md) | Ordem de construção, do MVP ao completo |

## Regras gerais

- O contrato da API (OpenAPI 3.1) é a fonte da verdade. Clientes são gerados a partir dele.
- Nenhuma regra de negócio vive só no cliente. Toda entrada é validada no servidor.
- Invariantes ficam no banco (FK, UNIQUE, CHECK), não só no código.
- Um jeito de fazer cada coisa. Mudou o padrão, muda em todo lugar.
- Mesmo nome para o mesmo conceito no banco, na API, no código e nestes documentos.
- Ao tomar uma decisão de arquitetura nova, atualize o documento correspondente.
