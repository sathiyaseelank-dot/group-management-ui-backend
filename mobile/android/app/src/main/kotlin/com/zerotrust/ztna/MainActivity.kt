package com.zerotrust.ztna

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import com.zerotrust.ztna.navigation.AppNavigation
import com.zerotrust.ztna.ui.theme.ZtnaTheme
import com.zerotrust.ztna.viewmodel.ZtnaViewModel

class MainActivity : ComponentActivity() {

    private val viewModel: ZtnaViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()

        // Handle deep link that arrived before the activity was created.
        handleDeepLink(intent)

        setContent {
            ZtnaTheme {
                AppNavigation(viewModel = viewModel)
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        // Handle deep link that arrives while the activity is already running
        // (launchMode = singleTask guarantees this path for ztna://callback).
        handleDeepLink(intent)
    }

    private fun handleDeepLink(intent: Intent?) {
        val data = intent?.data ?: return
        if (data.scheme == "ztna" && data.host == "callback") {
            val sessionCode = data.getQueryParameter("session_code") ?: return
            viewModel.completeLogin(sessionCode = sessionCode)
        }
    }
}
