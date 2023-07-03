package model

type DeploySmartContractRequest struct {
	SmartContractToken string
	DeployerAddress    string
	RBTAmount          float64
	QuorumType         int
	Comment            string
}
