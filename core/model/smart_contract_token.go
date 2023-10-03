package model

type DeploySmartContractRequest struct {
	SmartContractToken string  `json:"smartContractToken"`
	DeployerAddress    string  `json:"deployerAddr"`
	RBTAmount          float64 `json:"rbtAmount"`
	QuorumType         int     `json:"quorumType"`
	Comment            string  `json:"comment"`
}

type ExecuteSmartContractRequest struct {
	SmartContractToken string `json:"smartContractToken"`
	ExecutorAddress    string `json:"executorAddr"`
	QuorumType         int    `json:"quorumType"`
	Comment            string `json:"comment"`
	SmartContractData  string `json:"smartContractData"`
}
