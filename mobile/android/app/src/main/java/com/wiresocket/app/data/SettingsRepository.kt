package com.wiresocket.app.data

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

private val Context.dataStore: DataStore<Preferences> by preferencesDataStore(name = "settings")

class SettingsRepository(private val context: Context) {

    companion object {
        private val KEY_SERVER = stringPreferencesKey("server")
        private val KEY_USERNAME = stringPreferencesKey("username")
        // Note: Password should be stored in Android Keystore for production
        private val KEY_LAST_CONNECTED_SERVER = stringPreferencesKey("last_connected_server")
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

    suspend fun saveCredentials(server: String, username: String) {
        context.dataStore.edit { prefs ->
            prefs[KEY_SERVER] = server
            prefs[KEY_USERNAME] = username
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
    }
}
