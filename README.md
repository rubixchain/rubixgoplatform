# rubixgoplatform

The new Rubixgoplatform support command line options to run/configure the Rubix node. To run the application use the follwing format.

```
./rubixgoplatform <cmd> <options>

Use the following commands

                       -v : To get tool version

                       -h : To get help

                      run : To run the rubix core

                     ping : Use the command to ping the peer
```

Run Command
: To run the Rubix node use this command. 
```
./rubixgoplatform run -p node1 -n 0 -s -testNet

This following options are used to run the Rubix node
  -n uint
        Node number
  -p string
        Working directory path (default "./")
  -s    Start the core
  -testNet
        Run as test net
  -testNetKey string
        Test net key (default "testswarm.key")
```
Ping Command
: To ping any peer in network use this command.
```
./rubixgoplatform ping -peerID 12D3KooWKr8dEQiLXuKacxDCZiHePVEMpgjxk19C3QozuUVQcQHA -port <port>

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -peerID string
        Peerd ID
  -port string
        Server/Host port (default "20000")
```
Add Bootstrap Command
: To add bootstrap to node use this command.
```
./rubixgoplatform addbootstrap -peers /ip4/103.60.213.76/tcp/4001/p2p/QmR1VH6SsEN1wf4EmstxXtNMvR35KEetbBetiGWWKWavJ6

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -peers string
        Bootstrap peers, mutiple peers will be seprated by comma
  -port string
        Server/Host port (default "20000")
```
Remove Bootstrap Command
: To remove bootstrap from node use this command.
```
./rubixgoplatform removebootstrap -peers /ip4/103.60.213.76/tcp/4001/p2p/QmR1VH6SsEN1wf4EmstxXtNMvR35KEetbBetiGWWKWavJ6

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -peers string
        Bootstrap peers, mutiple peers will be seprated by comma
  -port string
        Server/Host port (default "20000")
```
Remove All Bootstrap Command
: To remove all bootstrap from node use this command.
```
./rubixgoplatform removeallbootstrap

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -port string
        Server/Host port (default "20000")
```
Get All Bootstrap Command
: To get all bootstrap from node use this command.
```
./rubixgoplatform getallbootstrap

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -port string
        Server/Host port (default "20000")
```
Create DID Command
: To create DID use this command.
```
./rubixgoplatform createdid

This following options are used for this command
  -port string
        Server/Host port (default "20000")
  -didType int
        DID type (0-Basic Mode, 1-Standard Mode, 2-Wallet Mode) (default 0)
  -didSecret string
        DID secret (default "My DID Secret")
  -privPWD string
        Private key password (default "mypassword")
  -quorumPWD string
        Quroum key password (default "mypassword")
  -imgFile string
        Image file to create DID (Must be 256x256 PNG image) (default "image.png")
  -didImgFile string
        DID image file name (default "did.png")
  -privImgFile string
        DID private share image file name (default "pvtShare.png")
  -pubImgFile string
        DID public share image file name (default "pubShare.png")
  -privKeyFile string
        DID private key file name (default "pvtKey.pem")
  -pubKeyFile string
        DID public key file name (default "pubKey.pem")
  -fp forcepassword
        This flag prompts to enter the password in terminal
```
Get All DID Command
: To get all DID use this command.
```
./rubixgoplatform getalldid

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -port string
        Server/Host port (default "20000")
```
To Register DID Command
: To register DID & PeerID map on the network use this command.
```
./rubixgoplatform registerdid 

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -port string
        Server/Host port (default "20000")
  -did string
        DID address (default "")
```
To Add Quorum List
: To add quorum list use this command.
```
./rubixgoplatform addquorum

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -port string
        Server/Host port (default "20000")
  -quorumList string
        quorum list file name (default "quorumlist.json")
```
To Get All Quorum List
: To get all quorum list use this command.
```
./rubixgoplatform getallquorum

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -port string
        Server/Host port (default "20000")
```
To Remove All Quorum List
: To remove all quorum list use this command.
```
./rubixgoplatform removeallquorum

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -port string
        Server/Host port (default "20000")
```
To Setup Quorum
: To setup quorum use this command. This setup quorum by providn quorum private key password.
```
./rubixgoplatform setupquorum

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -port string
        Server/Host port (default "20000")
  -quorumPWD string
        Quroum key password (default "mypassword")
  -fp forcepassword
        Enter the  Quroum key password in terminal
```
To Setup Service Command
: To setup service on the node use this command.
```
./rubixgoplatform setupservice 

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -port string
        Server/Host port (default "20000")
  -srvName string
        Service name (default "explorer_service")
  -dbAddress string
        Database address (default "localhost")
  -dbName string
        Explorer database name (default "ExplorerDB")
  -dbPassword string
        Database password (default "password")
  -dbPort string
        Database port number (default "1433")
  -dbType string
        DB Type, supported database are SQLServer, PostgressSQL, MySQL & Sqlite3 (default "SQLServer")
  -dbUsername string
        Database username (default "sa")
```
To Generate Test RBT Command
: To generate test RBT on the node use this command.
```
./rubixgoplatform generatetestrbt 

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -port string
        Server/Host port (default "20000")
  -did string
        DID address (default "")
  -numTokens int
        Number tokens to be generated (default 1)
  -fp 
        Force password to be entered on the terminal
  -privPWD string
        Private key password (default "mypassword")
  -privImgFile string
        DID private share image file name (default "pvtShare.png")
  -privKeyFile string
        DID private key file name (default "pvtKey.pem")
```
To Transfer RBT Command
: To trasnfer RBT on the node use this command.
```
./rubixgoplatform transferrbt 

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -port string
        Server/Host port (default "20000")
  -senderAddr string
        Sender address (default "")
  -receiverAddr string
        Receiver address (default "")
  -rbtAmount float
        RBT amount to trasnfered (default 0.0)
  -transComment string
        Transfer comment (default "Test tranasaction")
  -transType int
        Transaction type (default 2)
  -fp 
        Force password to be entered on the terminal
  -privPWD string
        Private key password (default "mypassword")
  -privImgFile string
        DID private share image file name (default "pvtShare.png")
  -privKeyFile string
        DID private key file name (default "pvtKey.pem")
```
To Get Account Info Command
: To get account information on the node use this command.
```
./rubixgoplatform getaccountinfo 

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -port string
        Server/Host port (default "20000")
  -did string
        DID address (default "")
```
To Dump Token Chain Command
: To dump token chain on the node use this command.
```
./rubixgoplatform dumptokenchain 

This following options are used for this command
  -addr string
        Server/Host Address (default "localhost")
  -port string
        Server/Host port (default "20000")
  -token string
        Token address (default "")
```
To Migrate Existing Java Node to RubixGo
: To dump token chain on the node use this command.
```
./rubixgoplatform migratenode 

This following options are used for this command
  -port string
        Server/Host port (default "20000")
   -fp 
        Force password to be entered on the terminal
```