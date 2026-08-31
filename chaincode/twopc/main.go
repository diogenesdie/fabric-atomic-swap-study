package main

import (
	"log"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

func main() {
	cc, err := contractapi.NewChaincode(&SmartContract{})
	if err != nil {
		log.Panicf("não foi possível criar o chaincode twopc: %v", err)
	}
	if err := cc.Start(); err != nil {
		log.Panicf("não foi possível iniciar o chaincode twopc: %v", err)
	}
}
