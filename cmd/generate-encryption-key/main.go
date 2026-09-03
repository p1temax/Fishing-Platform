package main

import (
	"fmt"

	"fishing-platform-backend/utils"
)

func main() {
	encoded := utils.GenerateEncryptionKey()

	fmt.Println("Generated Encryption Key:")
	fmt.Println("========================")
	fmt.Println(encoded)
	fmt.Println()
	fmt.Println("Add this to your .env file:")
	fmt.Printf("ENCRYPTION_KEY=%s\n", encoded)
	fmt.Println()
	fmt.Println("IMPORTANT: Keep this key secure. If you lose it,")
	fmt.Println("you won't be able to decrypt your encrypted data.")
}
