package params

var (
	Block_Interval      = 5000   // 生成新的块间隔
	MaxBlockSize_global = 6000   // 该块包含最大数量的事务
	InjectSpeed         = 5000   // 交易注入速度
	TotalDataSize       = 100000 // TXS的总数
	BatchSize           = 7000   // supervisor读取一批TXS然后发送，它应该大于注入速度
	BrokerNum           = 10
	NodesInShard        = 2
	ShardNum            = 4
	DataWrite_path      = "./result/"                           // measurement data result output path
	LogWrite_path       = "./log"                               // log output path
	SupervisorAddr      = "127.0.0.1:18800"                     //supervisor ip address
	FileInput           = `./Transactions20161227-20170105.csv` //the raw BlockTransaction data path
)
