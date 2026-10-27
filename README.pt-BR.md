# RoomFlow API

[![CI](https://github.com/joaoeliezyo/RoomFlow-API/actions/workflows/ci.yml/badge.svg)](https://github.com/joaoeliezyo/RoomFlow-API/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/joaoeliezyo/RoomFlow-API)](https://github.com/joaoeliezyo/RoomFlow-API/releases/tag/v1.0.0) ![Go](https://img.shields.io/badge/Go-1.27-00ADD8) ![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-4169E1) ![OpenAPI](https://img.shields.io/badge/OpenAPI-3.1-6BA539)

**API REST para reservas de salas com segurança sob concorrência, desenvolvida em Go e PostgreSQL.**

[English](README.md) · **Português (Brasil)**

O RoomFlow API é um projeto de portfólio para gerenciar salas e reservas. Seu desafio central é evitar reservas sobrepostas **mesmo quando duas requisições chegam simultaneamente**. Em vez de depender de uma consulta prévia na aplicação, o PostgreSQL garante a regra por meio de uma restrição de exclusão parcial GiST.

## Destaques

- CRUD de salas: criar, listar, consultar, atualizar parcialmente e excluir.
- Reservas: criar, listar, consultar e cancelar sem perder o histórico.
- Impedimento de sobreposição entre reservas **ativas** da mesma sala diretamente no banco.
- Horários RFC3339 com fuso obrigatório, normalizados para UTC nas respostas.
- SQL explícito com `pgx/v5`, sem ORM ou framework HTTP externo.
- Testes HTTP e de integração PostgreSQL, testes concorrentes e CI no GitHub Actions.
- Contrato OpenAPI 3.1 e ambiente reproduzível com Docker Compose.

## Visão da arquitetura

```mermaid
flowchart TD
    Client[Cliente HTTP] --> HTTP[Rotas e handlers net/http]
    HTTP --> Validation[Validação e respostas JSON]
    Validation --> Rooms[Store de salas]
    Validation --> Reservations[Store de reservas]
    Rooms --> PG[(PostgreSQL 17)]
    Reservations --> PG
    PG --> Rules[FK, CHECK e exclusão GiST parcial]
```

Os handlers ficam em `internal/httpapi`; as consultas SQL estão em `internal/rooms` e `internal/reservations`; a composição das dependências ocorre em `cmd/api/main.go`. Leia [arquitetura e decisões técnicas](docs/architecture.pt-BR.md) para os fluxos, modelo de dados e testes.

## Por que a constraint importa

Duas requisições concorrentes com intervalos sobrepostos para a mesma sala não podem ser aceitas simultaneamente. A regra está no PostgreSQL:

```sql
CONSTRAINT reservations_no_overlap EXCLUDE USING gist (
    room_id WITH =,
    tstzrange(starts_at, ends_at, '[)') WITH &&
) WHERE (cancelled_at IS NULL)
```

O intervalo `[)` inclui o início e exclui o fim. Assim, **09h–10h** e **10h–11h** podem coexistir. O cancelamento preenche `cancelled_at`, preserva o registro e libera o período. Salas com qualquer histórico de reservas, inclusive canceladas, não podem ser excluídas.

## Tecnologias

Go 1.27 · `net/http` · `pgx/v5` · PostgreSQL 17 · Docker / Docker Compose · Go `testing` · GitHub Actions · OpenAPI 3.1.

## Início rápido com Docker Compose

Requisitos: Docker e Docker Compose. Em um **banco novo**, aplique as migrations na ordem:

```sh
docker compose up -d --wait db
docker compose exec -T db psql -U roomflow -d roomflow -v ON_ERROR_STOP=1 -f /migrations/001_create_rooms.up.sql
docker compose exec -T db psql -U roomflow -d roomflow -v ON_ERROR_STOP=1 -f /migrations/002_create_reservations.up.sql
docker compose up -d --build api
curl -i http://localhost:8080/healthz
```

Se a 001 já estiver aplicada, execute apenas a 002. As migrations são **manuais**, não são aplicadas ao iniciar a API e não devem ser executadas duas vezes. As credenciais e portas do Compose destinam-se ao desenvolvimento local.

### Exemplos de requisição

Crie uma sala (guarde o ID retornado no header `Location`):

```sh
curl -i -X POST http://localhost:8080/rooms \
  -H 'Content-Type: application/json' \
  -d '{"name":"Sala Azul","capacity":8}'
```

Crie uma reserva (substitua `1` pelo ID da sala):

```sh
curl -i -X POST http://localhost:8080/reservations \
  -H 'Content-Type: application/json' \
  -d '{"room_id":1,"starts_at":"2026-11-10T09:00:00Z","ends_at":"2026-11-10T10:00:00Z"}'
```

Uma reserva ativa sobreposta na mesma sala retorna `409 Conflict`:

```json
{"error":{"code":"reservation_conflict","message":"reservation overlaps an active reservation"}}
```

Cancele com `POST /reservations/{id}/cancel` e tente novamente o horário. Todos os contratos estão no [OpenAPI 3.1](docs/openapi.yaml).

## Endpoints

| Método | Rota | Finalidade |
| --- | --- | --- |
| `GET` | `/healthz` | Verificar API e conexão com banco |
| `POST` | `/rooms` | Criar sala (`201`, `Location`) |
| `GET` | `/rooms` | Listar salas |
| `GET` | `/rooms/{id}` | Consultar sala |
| `PATCH` | `/rooms/{id}` | Atualizar sala parcialmente |
| `DELETE` | `/rooms/{id}` | Excluir sala sem histórico de reservas |
| `POST` | `/reservations` | Criar reserva (`201`, `Location`; `409` em conflito) |
| `GET` | `/reservations` | Listar reservas, inclusive canceladas |
| `GET` | `/reservations/{id}` | Consultar reserva |
| `POST` | `/reservations/{id}/cancel` | Cancelar (`409` se já cancelada) |

Sucessos usam `{"data":...}` (exceto `204 No Content`), e erros usam `{"error":{"code":"...","message":"..."}}`. Campos JSON desconhecidos ou repetidos são rejeitados; os corpos de requisição têm limite de 1 MiB. Os schemas e códigos completos estão na [especificação](docs/openapi.yaml).

## Execução local sem Docker

Exige Go 1.27 e PostgreSQL. Crie o banco, aplique as migrations SQL usando `psql -v ON_ERROR_STOP=1 -f ...`, e configure as variáveis (veja [.env.example](.env.example)):

```sh
export DATABASE_URL='postgres://roomflow:roomflow@localhost:5432/roomflow?sslmode=disable'
export APP_PORT=8080
go run ./cmd/api
```

`DATABASE_URL` é obrigatória; `APP_PORT` usa 8080 por padrão. `.env.example` não é carregado automaticamente.

## Testes

A [CI executa em pushes e pull requests para `main`](.github/workflows/ci.yml), inicia o serviço PostgreSQL 17, aplica as duas migrations e executa `go test ./...`, `go vet ./...` e `go test -race ./...`. `REQUIRE_INTEGRATION_TESTS=1` evita que testes integrados sejam ignorados na CI por falta de conexão.

Para rodar os testes com banco temporário via Compose:

```sh
docker compose --profile test up -d --wait testdb
docker compose --profile test exec -T testdb psql -U roomflow -d roomflow_test -v ON_ERROR_STOP=1 -f /migrations/001_create_rooms.up.sql
docker compose --profile test exec -T testdb psql -U roomflow -d roomflow_test -v ON_ERROR_STOP=1 -f /migrations/002_create_reservations.up.sql
docker compose --profile test run --rm test
docker compose --profile test run --rm test go test -race ./...
docker compose --profile test run --rm test go vet ./...
docker compose --profile test down
```

`testdb` usa armazenamento temporário, separado do banco de desenvolvimento. **Nunca** use seu banco de desenvolvimento ou produção como `TEST_DATABASE_URL`. Sem essa variável, os testes de integração são ignorados fora da CI.

## Estrutura

```text
cmd/api/                 inicialização, composição e encerramento seguro
internal/config/         configuração por ambiente
internal/httpapi/        rotas, validações, handlers e testes HTTP
internal/rooms/          persistência de salas e testes
internal/reservations/   persistência de reservas e testes de concorrência
db/migrations/           migrations SQL de subida e reversão
docs/openapi.yaml        contrato OpenAPI 3.1
docs/architecture.pt-BR.md arquitetura e decisões técnicas
.github/workflows/ci.yml integração contínua
```

## Migrations e limitações

- Aplique as migrations manualmente e na ordem (001, depois 002), uma vez por banco. Reverter a 002 **apaga todas as reservas**; reverter a 001 **apaga todas as salas**. Faça backup antes. A extensão `btree_gist` permanece após reverter a 002.
- Para reverter a 002, pare a API e execute `docker compose exec -T db psql -U roomflow -d roomflow -v ON_ERROR_STOP=1 -f /migrations/002_create_reservations.down.sql`. Para reverter a 001, execute depois seu arquivo `.down.sql` correspondente.
- Este é um MVP, **não um serviço público pronto para produção**: não há autenticação, autorização, paginação, interface gráfica, duração máxima de reservas ou proibição de reservas no passado.

## Mais documentação

[Architecture (English)](docs/architecture.md) · [Arquitetura (Português)](docs/architecture.pt-BR.md) · [OpenAPI](docs/openapi.yaml) · [Changelog](CHANGELOG.md) · [Release v1.0.0](https://github.com/joaoeliezyo/RoomFlow-API/releases/tag/v1.0.0)
