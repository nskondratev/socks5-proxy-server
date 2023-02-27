import { createClient } from 'redis'

const redisClient = createClient({
  host: process.env.REDIS_HOST,
  port: process.env.REDIS_PORT,
  db: process.env.REDIS_DB
})

export default await redisClient.connect().then(() => redisClient)
