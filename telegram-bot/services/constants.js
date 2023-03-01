export const REDIS = Object.freeze({
    AUTH_USER_KEY: 'user_auth',
    DATA_USAGE_KEY: 'user_usage_data',
    AUTH_DATE_KEY: 'user_auth_date',
    ADMIN_USER_KEY: 'user_admin',
    USER_STATE: 'user_state'
})

export const USER_STATE = Object.freeze({
    IDLE: 'idle',
    CREATE_USER_ENTER_USERNAME: 'create_user_enter_username',
    CREATE_USER_ENTER_PASSWORD: 'create_user_enter_password',
    DELETE_USER_ENTER_USERNAME: 'delete_user_enter_username'
})
