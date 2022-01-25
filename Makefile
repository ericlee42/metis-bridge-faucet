build:
	rm -rf dist && mkdir dist
	go build -o ./dist .

goabigen:
	mkdir -p internal/goabi/metisl2
	abigen --abi ./abis/L2StandardBridge.json -pkg metisl2 --type L2StandardBridge --out internal/goabi/metisl2/L2StandardBridge.go
	abigen --abi ./abis/L2StandardERC20.json -pkg metisl2 --type L2StandardERC20 --out internal/goabi/metisl2/L2StandardERC20.go

createdb:
	docker exec -e MYSQL_PWD=passwd metisdb mysql -uroot -e 'create database if not exists metis;'

dropdb:
	docker exec -e MYSQL_PWD=passwd metisdb mysql -uroot -e 'drop database if exists metis;'

dbshell:
	docker exec -it metisdb mysql -u root -D metis -p
