FROM node:10.14.2-alpine

RUN apk add --no-cache git python g++ make

ADD . /app

WORKDIR /app

RUN npm install pm2 -g

RUN npm i --only=production

CMD ["pm2", "start", "app.js", "--no-daemon", "--name", "proxy"]
