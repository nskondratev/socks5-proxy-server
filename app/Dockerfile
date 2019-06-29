FROM node:10.16.0-jessie

ADD . /app

WORKDIR /app

RUN npm install pm2 -g

RUN npm i --production --unsafe

CMD ["pm2", "start", "app.js", "--no-daemon", "--name", "proxy"]
