FROM node:8.11.1-alpine

RUN apk add --no-cache python g++ make

ADD . /telegram-bot

WORKDIR /app

RUN npm install pm2 -g

RUN npm i --only=production

CMD ["pm2", "start", "app.js", "--no-daemon", "--name", "telegram-bot"]
