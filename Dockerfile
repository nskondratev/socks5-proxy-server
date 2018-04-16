FROM node:8.11.1-alpine

RUN apk add --no-cache git python g++ make

#RUN ["git", "--version"]

ADD ./app /app

WORKDIR /app

RUN npm install pm2 -g

RUN npm i --only=production

CMD ["pm2", "start", "app.js", "--no-daemon", "--name", "proxy"]
