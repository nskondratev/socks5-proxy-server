FROM node:18.17.1

ADD . /app

WORKDIR /app

RUN npm install pm2 -g

RUN npm i --production --unsafe

CMD ["pm2", "start", "app.js", "--no-daemon", "--name", "telegram-bot"]
