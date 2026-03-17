package com.zerotrust.ztna

import android.app.Application

class ZtnaApplication : Application() {

    override fun onCreate() {
        super.onCreate()
        // UniFFI loads libztna.so via System.loadLibrary at class-init time,
        // so no explicit load call is required here. The generated ztna.kt
        // handles this internally.
    }
}
