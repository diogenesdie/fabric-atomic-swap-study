package main

// Conexão com as duas redes Fabric.
//
// Reaproveitamos a configuração e as wallets que scripts/03-setup-htlc.sh já
// preparou para a go-cli do Weaver, em vez de manter um segundo cadastro de
// identidades. Assim o orquestrador e a CLI operam sobre as mesmas contas.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric-sdk-go/pkg/core/config"
	"github.com/hyperledger/fabric-sdk-go/pkg/gateway"
)

// NetworkConfig descreve uma das redes, no formato do config.json da go-cli.
type NetworkConfig struct {
	ConnProfilePath string `json:"connProfilePath"`
	RelayEndpoint   string `json:"relayEndpoint"`
	MspID           string `json:"mspId"`
	ChannelName     string `json:"channelName"`
	Chaincode       string `json:"chaincode"`
}

// Config mapeia o config.json inteiro (network1 e network2).
type Config map[string]NetworkConfig

// LoadConfig lê o config.json usado pela go-cli.
func LoadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("não foi possível ler %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("config.json inválido em %s: %w", path, err)
	}
	for name, nc := range cfg {
		if nc.ConnProfilePath == "" || nc.ChannelName == "" || nc.Chaincode == "" {
			return nil, fmt.Errorf("configuração incompleta para a rede %q", name)
		}
	}
	return cfg, nil
}

// Session é uma conexão aberta a um chaincode, sob uma identidade específica.
type Session struct {
	Network string
	User    string
	CertB64 string // certificado do usuário, em base64 — o HTLC identifica as
	// partes por certificado, não por nome
	Contract *gateway.Contract
	gw       *gateway.Gateway
}

// Close libera a conexão.
func (s *Session) Close() {
	if s.gw != nil {
		s.gw.Close()
	}
}

// Connect abre uma sessão numa rede sob a identidade informada.
//
// A identidade precisa existir na wallet — quem a cria é
// scripts/03-setup-htlc.sh. Aqui apenas a carregamos: registrar usuários no
// meio de um experimento distorceria as medições de latência.
func Connect(cfg Config, walletRoot, network, user string) (*Session, error) {
	nc, ok := cfg[network]
	if !ok {
		return nil, fmt.Errorf("rede desconhecida: %s", network)
	}

	// Sem isto o service discovery devolve os endereços INTERNOS dos peers
	// (peer0.org1.network1.com:7051), que não resolvem a partir do host, e toda
	// consulta falha com "Endorser Client Status Code: (2)". A variável faz o
	// fabric-sdk-go reescrever os endereços descobertos para localhost.
	// É o que a go-cli do Weaver faz em FabricHelper() — e não está documentado
	// em lugar óbvio.
	if err := os.Setenv("DISCOVERY_AS_LOCALHOST", "true"); err != nil {
		return nil, fmt.Errorf("não foi possível definir DISCOVERY_AS_LOCALHOST: %w", err)
	}

	wallet, err := gateway.NewFileSystemWallet(filepath.Join(walletRoot, network))
	if err != nil {
		return nil, fmt.Errorf("não foi possível abrir a wallet de %s: %w", network, err)
	}
	if !wallet.Exists(user) {
		return nil, fmt.Errorf(
			"identidade %q ausente na wallet de %s — rode ./scripts/03-setup-htlc.sh",
			user, network)
	}

	gw, err := gateway.Connect(
		gateway.WithConfig(config.FromFile(nc.ConnProfilePath)),
		gateway.WithIdentity(wallet, user),
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar em %s como %s: %w", network, user, err)
	}

	net, err := gw.GetNetwork(nc.ChannelName)
	if err != nil {
		gw.Close()
		return nil, fmt.Errorf("canal %s indisponível em %s: %w", nc.ChannelName, network, err)
	}

	cert, err := certificateOf(wallet, user)
	if err != nil {
		gw.Close()
		return nil, err
	}

	return &Session{
		Network:  network,
		User:     user,
		CertB64:  cert,
		Contract: net.GetContract(nc.Chaincode),
		gw:       gw,
	}, nil
}

// certificateOf extrai o certificado X.509 da identidade, em base64.
//
// É esse valor que o chaincode grava como dono do ativo e que as chamadas de
// HTLC usam para designar locker e recipient.
func certificateOf(wallet *gateway.Wallet, user string) (string, error) {
	id, err := wallet.Get(user)
	if err != nil {
		return "", fmt.Errorf("não foi possível ler a identidade %q: %w", user, err)
	}
	x509id, ok := id.(*gateway.X509Identity)
	if !ok {
		return "", fmt.Errorf("identidade %q não é X.509", user)
	}
	return base64.StdEncoding.EncodeToString([]byte(x509id.Certificate())), nil
}
