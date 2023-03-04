import { createClient } from 'redis'

const redisClient = createClient({
  socket: {
    host: process.env.REDIS_HOST,
    port: process.env.REDIS_PORT,
  },
  database: process.env.REDIS_DB
})

export default await redisClient.connect().then(() => redisClient)
