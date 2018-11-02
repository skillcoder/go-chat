# go-chat
CLI Client/Server chat in go with gRPC transport

### Install
go get github.com/skillcoder/go-chat
(make sure you have set GOPATH and GOOS)
cd ${GOPATH}/src/github.com/skillcoder/go-chat
go get -u github.com/golang/protobuf/protoc-gen-go
export PATH=$PATH:$GOPATH/bin

cd server
glide update

cd ../client
glide update

### Deps
#### Freebsd
sudo pkg install go
sudo pkg install go-glide
sudo pkg install protobuf
export GOOS=freebsd

#### Ubuntu
sudo add-apt-repository ppa:maarten-fonville/protobuf
sudo apt-get update
sudo apt install protobuf-compiler
export GOOS=linux

### Build
make proto

###Run server
`cd server`
```
make
bin/linux/go-chat-server [-v] [-l 0.0.0.0:8000]
```

###Run client
`cd client`
```
make
bin/linux/go-chat-client [-v] [-h 127.0.0.1:8000] [-u Username]
```

