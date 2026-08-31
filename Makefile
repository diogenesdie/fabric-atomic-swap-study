# HTLC vs 2PC — orquestração de topo
#
# As etapas seguem a ordem: prereqs -> setup -> networks-up -> spike
# -> deploy-2pc -> experiments -> analyze

SHELL := /bin/bash
.DEFAULT_GOAL := help

# Diretório do clone do Cacti/Weaver (fora do versionamento)
CACTI_DIR ?= $(CURDIR)/cacti
# Testbed de duas redes Fabric dentro do monorepo
WEAVER_NET_DIR := $(CACTI_DIR)/weaver/tests/network-setups/fabric/dev
# Chaincode de aplicação usado no braço HTLC
HTLC_CHAINCODE ?= simpleasset
# Repetições por cenário. N=10 é o mínimo para falar de frequência.
N ?= 10
# Usa o venv da análise quando existe; senão o python do sistema.
PY := $(shell [ -x ./.venv/bin/python ] && echo ./.venv/bin/python || echo python3)

.PHONY: help
help: ## Lista os alvos disponíveis
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------- infraestrutura

.PHONY: prereqs
prereqs: ## Verifica Docker, Go, make e memória disponível
	./scripts/00-prereqs.sh

.PHONY: setup
setup: prereqs ## Clona o Hyperledger Cacti/Weaver na versão pinada
	./scripts/01-clone-weaver.sh

.PHONY: networks-up
networks-up: ## Sobe as duas redes Fabric com interop-CC e chaincode de aplicação
	./scripts/02-networks-up.sh

.PHONY: networks-down
networks-down: ## Derruba as duas redes Fabric
	./scripts/networks-down.sh

.PHONY: clean-networks
clean-networks: ## Derruba as redes e apaga volumes/estado (reset completo)
	./scripts/networks-down.sh --clean

# ---------------------------------------------------------------- braço HTLC

.PHONY: setup-htlc
setup-htlc: ## Corrige os perfis, registra usuários e popula os ativos do braço HTLC
	./scripts/03-setup-htlc.sh

.PHONY: spike
spike: ## Gate de viabilidade: executa um swap HTLC completo fim a fim
	./scripts/04-spike-htlc.sh

.PHONY: spike-clean
spike-clean: clean-networks networks-up setup-htlc spike ## Ciclo completo do zero

.PHONY: htlc-demo
htlc-demo: ## Executa o orquestrador HTLC instrumentado (caso sem falha)
	go run ./apps/htlc-orchestrator -scenario=B1

# ---------------------------------------------------------------- braço 2PC

.PHONY: deploy-2pc
deploy-2pc: ## Empacota, instala, aprova e efetiva o chaincode twopc nas duas redes
	./scripts/05-deploy-2pc.sh

.PHONY: 2pc-demo
2pc-demo: ## Executa o coordenador 2PC (caso sem falha)
	go run ./apps/coordinator -scenario=B2

.PHONY: 2pc-recover
2pc-recover: ## Retoma as transações pendentes no write-ahead log
	go run ./apps/coordinator -recover

# ---------------------------------------------------------------- experimentos

.PHONY: experiments
experiments: ## Roda a matriz completa de cenários com injeção de falha
	./experiments/runner.sh --all --repeat=$(N)

.PHONY: scenarios
scenarios: ## Lista os cenários disponíveis
	./experiments/runner.sh --list

.PHONY: analyze
analyze: ## Agrega os CSVs e gera as figuras e tabelas do artigo
	$(PY) analysis/analyze.py

.PHONY: summary
summary: ## Resumo rápido dos resultados no terminal
	$(PY) analysis/summarize.py

.PHONY: venv
venv: ## Cria o ambiente Python da análise
	python3 -m venv .venv && ./.venv/bin/pip install -q -r analysis/requirements.txt

# ---------------------------------------------------------------- qualidade

.PHONY: test
test: ## Testes unitários do chaincode e das aplicações Go
	go test ./...
	# O chaincode é um módulo Go separado (é empacotado e compilado pelo peer),
	# então não é alcançado pelo ./... da raiz.
	cd chaincode/twopc && go test ./...

.PHONY: fmt
fmt: ## Formata o código Go
	gofmt -s -w chaincode apps
