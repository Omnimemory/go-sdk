package main

import (
	"context"
	"fmt"
	"log"
	"os"

	memaura "github.com/Omnimemory/go-sdk"
)

func main() {
	apiKey := os.Getenv("MEMAURA_API_KEY")
	if apiKey == "" {
		log.Fatal("MEMAURA_API_KEY is required")
	}

	memory, err := memaura.NewMemory(memaura.Config{APIKey: apiKey, DeviceNo: "go-example"})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	ack, err := memory.Add(ctx, memaura.AddInput{
		CommitID: "go-example-1",
		Turns: []memaura.Turn{{
			Content: "The customer prefers weekly Monday status updates.",
			Role:    memaura.RoleUser,
		}},
	})
	if err != nil {
		log.Fatal(err)
	}
	if _, err := memory.Jobs().Wait(ctx, ack.JobID); err != nil {
		log.Fatal(err)
	}

	result, err := memory.SearchHybrid(ctx, memaura.SearchInput{Query: "When does the customer want updates?", Limit: 3})
	if err != nil {
		log.Fatal(err)
	}
	for _, evidence := range result.EvidenceDetails {
		fmt.Println(evidence.Text)
	}
}
