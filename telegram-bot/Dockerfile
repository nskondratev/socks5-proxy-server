FROM node:18.16.0

ADD . /app

WORKDIR /app

RUN npm install pm2 -g

RUN npm i --production --unsafe

CMD ["pm2", "start", "app.js", "--no-daemon", "--name", "telegram-bot"]
