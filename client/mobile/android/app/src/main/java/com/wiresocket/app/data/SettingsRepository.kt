package com.wiresocket.app.data

import android.content.Context
import android.content.SharedPreferences
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.runBlocking

private val Context.dataStore: DataStore<Preferences> by preferencesDataStore(name = "settings")

class SettingsRepository(private val context: Context) {

    companion object {
        private val KEY_SERVER = stringPreferencesKey("server")
        private val KEY_USERNAME = stringPreferencesKey("username")
        private val KEY_LAST_CONNECTED_SERVER = stringPreferencesKey("last_connected_server")
        private val KEY_REMEMBER_PASSWORD = stringPreferencesKey("remember_password")
        private const val ENCRYPTED_PREFS_NAME = "encrypted_credentials"
        private const val KEY_PASSWORD = "password"
    }

    private val encryptedPrefs: SharedPreferences by lazy {
        val masterKey = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()

        EncryptedSharedPreferences.create(
            context,
            ENCRYPTED_PREFS_NAME,
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM
        )
    }

    val serverFlow: Flow<String> = context.dataStore.data.map { prefs ->
        prefs[KEY_SERVER] ?: ""
    }

    val usernameFlow: Flow<String> = context.dataStore.data.map { prefs ->
        prefs[KEY_USERNAME] ?: ""
    }

    val lastConnectedServerFlow: Flow<String> = context.dataStore.data.map { prefs ->
        prefs[KEY_LAST_CONNECTED_SERVER] ?: ""
    }

    val rememberPasswordFlow: Flow<Boolean> = context.dataStore.data.map { prefs ->
        prefs[KEY_REMEMBER_PASSWORD] == "true"
    }

    fun getPassword(): String {
        return encryptedPrefs.getString(KEY_PASSWORD, "") ?: ""
    }

    fun getRememberPassword(): Boolean {
        return runBlocking {
            context.dataStore.data.first()[KEY_REMEMBER_PASSWORD] == "true"
        }
    }

    fun savePassword(password: String) {
        encryptedPrefs.edit().putString(KEY_PASSWORD, password).apply()
    }

    fun clearPassword() {
        encryptedPrefs.edit().remove(KEY_PASSWORD).apply()
    }

    suspend fun saveCredentials(server: String, username: String, password: String, rememberPassword: Boolean) {
        context.dataStore.edit { prefs ->
            prefs[KEY_SERVER] = server
            prefs[KEY_USERNAME] = username
            prefs[KEY_REMEMBER_PASSWORD] = rememberPassword.toString()
        }
        if (rememberPassword) {
            savePassword(password)
        } else {
            clearPassword()
        }
    }

    suspend fun saveLastConnectedServer(server: String) {
        context.dataStore.edit { prefs ->
            prefs[KEY_LAST_CONNECTED_SERVER] = server
        }
    }

    suspend fun clearCredentials() {
        context.dataStore.edit { prefs ->
            prefs.remove(KEY_SERVER)
            prefs.remove(KEY_USERNAME)
        }
        encryptedPrefs.edit().remove(KEY_PASSWORD).apply()
    }
}
