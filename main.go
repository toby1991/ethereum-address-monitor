package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// go run main.go -infura <your_infura_token_here> -address <address_to_monitor> -apikey <pushover_api_key> -user_key <pushover_user_key>
func main() {
	your_infura_token_here := flag.String("infura", "", "infura token")
	monitor_address := flag.String("address", "", "address to monitor")
	api_key := flag.String("apikey", "", "pushover api key")
	user_key := flag.String("user_key", "", "pushover user key")

	flag.Parse()

	client, err := ethclient.Dial("wss://mainnet.infura.io/ws/v3/" + *your_infura_token_here)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	targetAddress := common.HexToAddress(*monitor_address)
	fmt.Printf("Monitoring address: %s\n", targetAddress.Hex())

	headers := make(chan *types.Header)

	_, err = client.SubscribeNewHead(ctx, headers)
	if err != nil {
		log.Fatal(err)
	}

	for header := range headers {
		block, err := client.BlockByNumber(ctx, header.Number)
		if err != nil {
			log.Println(err)
			continue
		}

		for _, tx := range block.Transactions() {
			// Get the sender of the transaction
			from, err := getMessageSender(tx, tx.ChainId())
			if err != nil {
				log.Println(err)
				continue
			}

			if from == targetAddress {
				fmt.Printf("New outgoing transaction:\n")
				fmt.Printf("\tTx Hash: %s\n", tx.Hash().Hex())
				fmt.Printf("\tFrom: %s\n", from.Hex())
				fmt.Printf("\tTo: %s\n", tx.To().Hex())
				divisor := new(big.Float).SetInt(new(big.Int).SetUint64(params.Ether))
				ethValue := new(big.Float).Quo(new(big.Float).SetInt(tx.Value()), divisor)
				fmt.Printf("\tValue: %s\n", ethValue.String())

				err := sendPushNotification(*api_key, *user_key, fmt.Sprintf("%s -> %s : %s ETH", from.Hex(), tx.To().Hex(), ethValue.String()), ethValue.String()+" ETH", fmt.Sprintf("https://etherscan.io/tx/%s", tx.Hash().Hex()))
				if err != nil {
					log.Println(err)
					return
				}
			}
		}
	}
}

func getMessageSender(tx *types.Transaction, chainId *big.Int) (common.Address, error) {
	var signer types.Signer

	switch tx.Type() {
	case types.LegacyTxType: // 0x00
		signer = types.NewEIP155Signer(chainId)
	case types.AccessListTxType, types.DynamicFeeTxType: // 0x01 and 0x02 (EIP-1559)
		signer = types.LatestSignerForChainID(chainId)
	default:
		return common.Address{}, fmt.Errorf("transaction type not supported: %d", tx.Type())
	}

	return types.Sender(signer, tx)
}

type PushoverRequestBody struct {
	Token   string `json:"token"`
	User    string `json:"user"`
	Message string `json:"message"`
	Url     string `json:"url,omitempty"`
	Title   string `json:"title"`
}

func sendPushNotification(apiKey, userKey, message, title, url string) error {
	requestBody := &PushoverRequestBody{
		Token:   apiKey,
		User:    userKey,
		Message: message,
		Url:     url,
		Title:   title,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	resp, err := http.Post("https://api.pushover.net/1/messages.json", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return errors.New("failed to read response body")
		}
		return errors.New("failed to send push notification : " + string(body))
	}

	return nil
}
