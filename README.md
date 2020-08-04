# GOLANG leader election based on etcd
Leader election tool with local network check.

# Usage
go build -o election election.go

./election --help
Usage of ./election:
  -e string
        etcd host(:port) (default "http://127.0.0.1:2379")
  -h string
        hostname (default "host1")
  -l string
        listen port (default "8000")
  -s string
        service name (default "service")

./election -s "service name" -h "hostname" -e "192.168.1.10:2379,192.168.1.11:2379,192.168.1.12:2379," -l "8000"

#curl http://localhost:8000
#{"service":"service name","host":"hostname","status":1}

status = 1: leader
status = 0: follower
status = -1: disconnected from etcd cluster (or local network failure)
