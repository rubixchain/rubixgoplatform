package model

type DeploySmartContractRequest struct {
	SmartContractToken string
	DeployerAddress    string
	RBTAmount          float64
	QuorumType         int
	Comment            string
}

type ExecuteSmartContractRequest struct {
	SmartContractToken string
	ExecutorAddress    string
	QuorumType         int
	Comment            string
	SmartContractData  string
}
