# Análise da Configuração de Modelos

Esta análise descreve a configuração sugerida em
`configs/smart-model-router_all.yaml`.

Ela usa `strategy: decision_engine`: o plugin classifica cada solicitação como
`Planning`, `Coding`, `Review`, `Testing`, `Debug`, `Security`,
`Documentation` ou `Performance`, extrai sinais de contexto/complexidade e
aplica a regra mais específica em `routes:`.

`models:` continua sendo a fonte autoritativa. Uma rota só pode selecionar um
modelo declarado nessa lista; seus metadados também sustentam o fallback local
determinístico quando nenhuma regra for aplicável.

## Papel de Cada Modelo

| Modelo | Papel configurado | Seleção direta |
| --- | --- | --- |
| `claude-opus-4-8` | Máxima qualidade para decisões sensíveis e raciocínio amplo | Todo `Security`; `Planning` com complexidade `>= 50` |
| `claude-sonnet-5` | Código, revisão, performance e alterações grandes | `Coding` com diff e ao menos 4 arquivos; `Review` ou `Performance` com complexidade `>= 50` |
| `claude-haiku-4-5` | Documentação simples, rápida e de baixo custo | `Documentation` com complexidade `low`, exceto Markdown simples |
| `gpt-5.6-sol` | Planejamento e raciocínio de alta qualidade | `Planning` sem complexidade média ou alta detectada |
| `gpt-5.6-terra` | Caminho principal para trabalho técnico complexo | `Coding`, `Review`, `Testing` e `Debug` com complexidade `>= 50`; `Review` menos complexo |
| `gpt-5.6-luna` | Trabalho geral intermediário, documentação elaborada e performance não crítica | `Performance` abaixo de 50; `Documentation` com complexidade `>= 25` |
| `gpt-5.4-mini` | Modelo econômico para solicitações triviais não classificadas | Solicitação de complexidade `low` sem rota mais específica |
| `glm-5.2` | Planejamento e análise de nível médio | `Planning` com complexidade `medium` |
| `kimi-k2.7-code` | Implementação, testes e depuração comuns | `Coding`, `Testing` e `Debug` abaixo de complexidade 50 |
| `gemini-3-flash` | Documentação geral rápida | `Documentation` sem regra mais específica |
| `gemini-3.1-flash-lite` | Markdown simples, rápido e econômico | `Documentation` + linguagem `markdown` + complexidade `low` |

## Precedência das Regras

- A regra com mais condições em `when:` vence; empates preservam a ordem de
  declaração no arquivo.
- `Security` é sempre direcionado ao Opus, sem depender da complexidade.
- Alterações de código com diff e quatro ou mais arquivos vão para Sonnet,
  pois a regra específica tem precedência sobre a regra geral de `Coding`.
- Markdown simples satisfaz a regra do Haiku e a do Gemini Lite, mas a regra do
  Lite possui três condições (`task`, `language`, `complexity`) e vence.
- A complexidade `>= 50` distingue, em geral, tarefas de maior impacto para
  Terra, Sonnet ou Opus das tarefas normais direcionadas a Kimi ou Luna.

## Comportamento Operacional

- `keep_same_model_per_session: false` faz a política ser avaliada em cada
  solicitação. Assim, uma conversa pode planejar no Opus e depurar no Terra ou
  Kimi na solicitação seguinte.
- O cache continua ativo por 24 horas. Solicitações idênticas podem reutilizar
  uma decisão anterior antes de executar a política novamente.
- O classificador LLM está desativado: `decision_engine` usa somente a política
  local explícita e não chama classificadores.
- Não existe regra catch-all. Solicitações ambíguas, não classificadas ou sem
  uma regra válida caem no fallback determinístico baseado em `models:`.
- `capabilities`, `cost` e `quality` são metadados de fallback e devem ser
  calibrados conforme qualidade observada, latência e cobrança reais.
