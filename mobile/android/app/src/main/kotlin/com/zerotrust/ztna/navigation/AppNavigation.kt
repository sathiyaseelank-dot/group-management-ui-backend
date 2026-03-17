package com.zerotrust.ztna.navigation

import android.content.Context
import android.net.Uri
import androidx.browser.customtabs.CustomTabsIntent
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.platform.LocalContext
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import com.zerotrust.ztna.ui.login.LoginScreen
import com.zerotrust.ztna.ui.resources.ResourcesScreen
import com.zerotrust.ztna.ui.settings.SettingsScreen
import com.zerotrust.ztna.viewmodel.UiState
import com.zerotrust.ztna.viewmodel.ZtnaViewModel

object Routes {
    const val LOGIN = "login"
    const val RESOURCES = "resources"
    const val SETTINGS = "settings"
}

fun openChromeCustomTab(context: Context, url: String) {
    val intent = CustomTabsIntent.Builder()
        .setShowTitle(true)
        .build()
    intent.launchUrl(context, Uri.parse(url))
}

@Composable
fun AppNavigation(viewModel: ZtnaViewModel) {
    val navController = rememberNavController()
    val uiState by viewModel.uiState.collectAsState()
    val context = LocalContext.current

    // Drive navigation from ViewModel state.
    LaunchedEffect(uiState) {
        when (uiState) {
            is UiState.LoggedOut, is UiState.Error ->
                navController.navigate(Routes.LOGIN) {
                    popUpTo(0) { inclusive = true }
                }
            is UiState.AwaitingCallback -> {
                val authUrl = (uiState as UiState.AwaitingCallback).authUrl
                openChromeCustomTab(context, authUrl)
                // Stay on Login screen; deep link will trigger completeLogin
            }
            is UiState.LoggedIn ->
                navController.navigate(Routes.RESOURCES) {
                    popUpTo(0) { inclusive = true }
                }
            else -> Unit
        }
    }

    NavHost(navController = navController, startDestination = Routes.LOGIN) {
        composable(Routes.LOGIN) {
            LoginScreen(viewModel = viewModel)
        }
        composable(Routes.RESOURCES) {
            ResourcesScreen(
                viewModel = viewModel,
                onOpenSettings = { navController.navigate(Routes.SETTINGS) }
            )
        }
        composable(Routes.SETTINGS) {
            SettingsScreen(
                viewModel = viewModel,
                onBack = { navController.popBackStack() }
            )
        }
    }
}
