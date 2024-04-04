package main

func main() {
	// conn, err := grpc.Dial("localhost:10500", grpc.WithInsecure())
	// if err != nil {
	// 	fmt.Printf("Failed to dial")
	// 	return
	// }
	// defer conn.Close()

	// client := protos.NewRubixServiceClient(conn)
	// ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	// defer cancel()

	// ib, err := ioutil.ReadFile("image.png")

	// if err != nil {
	// 	fmt.Println("Image file not found")
	// 	return
	// }

	// Add token to gRPC Request.
	// ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token.AccessToken)

	// response, err := client.CreateDID(ctx, &protos.CreateDIDReq{DidMode: int32(did.ChildDIDMode), MasterDid: "bafybmicf4nnsy6bfuyojosn4tq76i6buheepfhylc6f3ra3sm4c7s7cuma", PrivKeyPwd: "mypassword", Secret: "testsecret"})

	// if err != nil {
	// 	fmt.Printf("faield to create did, %s\n", err.Error())
	// 	return
	// }
	// fmt.Printf("DID created : %s\n", response.Did)
	runCommand()
}
