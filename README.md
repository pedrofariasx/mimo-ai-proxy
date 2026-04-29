# Mimo AI Gateway & Proxy (Go)

Gateway avançado e Proxy API de alta performance para o ecossistema Mimo AI da Xiaomi, projetado para fornecer uma ponte robusta entre as capacidades do Mimo e o padrão de mercado OpenAI.

## Visão Geral

O **Mimo AI Proxy** não é apenas uma camada de tradução; é um gateway completo que gerencia sessões, otimiza o uso de contexto, provê persistência de dados e oferece ferramentas de monitoramento em tempo real. Ele permite que desenvolvedores utilizem o Mimo AI como se fosse um modelo nativo da OpenAI, com suporte total a recursos avançados como streaming, reasoning e tool calling.

## Funcionalidades Principais

- **OpenAI Standard Gateway**: Implementação completa dos endpoints `/v1/chat/completions` e `/v1/models`.
- **Inteligência de Sessão**: 
  - Detecção automática de conversas via fingerprinting de mensagens.
  - Sincronização bi-direcional com o histórico oficial da Xiaomi.
  - Persistência local robusta em SQLite.
- **Otimização de Contexto (Context Mastery)**:
  - Suporte a contextos massivos de até **1 Milhão de Tokens**.
  - Gerenciamento inteligente de payload, enviando apenas deltas quando uma sessão é identificada, garantindo estabilidade e performance.
- **AI-Native Features**:
  - **Reasoning (Thinking)**: Extração nativa de blocos `<think>` para o campo `reasoning_content`.
  - **Sequential Tool Calling**: Orquestração de múltiplas chamadas de ferramentas em sequência.
  - **Web Search**: Ativação dinâmica de busca na web via modelo ou parâmetro de requisição.
- **Infraestrutura e Operações**:
  - **Load Balancing & Account Rotation**: Rotação automática de múltiplas contas Xiaomi para alta disponibilidade.
  - **Live Dashboard**: Interface web integrada para monitoramento de uptime, latência upstream e consumo de tokens por conta.
  - **Direct Proxy**: Acesso de baixo nível ao endpoint original da Xiaomi via `/open-apis/bot/chat`.

## Configuração

1. **Requisitos**: Go 1.24+ ou Docker.

2. **Variáveis de Ambiente**: Configure o `.env` (use `[.env.example](.env.example)` como base).
   ```env
   # Múltiplos valores permitidos para Load Balancing:
   SERVICE_TOKENS="token1,token2"
   USER_IDS="id1,id2"
   XIAOMI_CHATBOT_PHS="ph1,ph2"
   
   # Segurança e Rede:
   PORT=3000
   API_KEY="sua_chave_secreta"
   CORS_ORIGIN="*"
   ```

## Como usar

### Docker (Recomendado)
```bash
docker-compose up -d
```

### Manualmente
```bash
go mod tidy
go run main.go
```

### Exemplo de Integração
```bash
curl http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer sua_chave" \
  -d '{
    "model": "mimo-v2.5-pro",
    "messages": [{"role": "user", "content": "Explique a teoria da relatividade."}],
    "stream": true
  }'
```

## Arquitetura de Dados

O gateway utiliza uma base **SQLite** local (`data/history.db`) para garantir que as conversas sejam mantidas mesmo entre reinicializações, permitindo consultas rápidas ao histórico e sincronização sob demanda com a nuvem da Xiaomi.

## Licença

MIT
