 docker run -i -t --env=GETTING_MESSAGES_WEBHOOK=https://172.20.8.70:4433/webhook/ --env=WAPI_HOST=0.0.0.0:4444 -p 4444:4444 erema/wapi:alpha