go build -o build/wapi remote_interaction/functions.go remote_interaction/api.go

docker build . -t erema/wapi:alpha

docker push  erema/wapi:alpha