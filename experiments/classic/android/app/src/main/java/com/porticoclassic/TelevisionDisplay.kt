package com.porticoclassic

import android.app.Application
import android.app.UiModeManager
import android.content.Context
import android.content.res.Configuration
import android.hardware.display.DisplayManager
import android.util.DisplayMetrics
import android.view.Display
import com.facebook.react.internal.featureflags.ReactNativeFeatureFlags
import com.facebook.react.internal.featureflags.ReactNativeFeatureFlagsOverrides_RNOSS_Stable_Android
import com.facebook.react.internal.featureflags.ReactNativeFeatureFlagsProvider
import com.facebook.react.uimanager.DisplayMetricsHolder

/**
 * The television design space.
 *
 * The shared screens lay a television out at 1920×1080 points, which is what tvOS reports. Every
 * Android television reports 960×540 dp instead (xhdpi at 1080p, xxxhdpi at 4K), so the same
 * layout would be drawn at twice its size. Rather than teach every screen a second scale, the
 * app measures a television in the design space: one unit is 1/1920 of the screen's width. On a
 * 1080p panel that is one pixel, exactly as on an Apple TV HD.
 *
 * Phones and tablets are untouched.
 */
internal object TelevisionDisplay {
  private const val DESIGN_WIDTH = 1920f

  /** The design-space density, measured once when the application starts; null on a phone or tablet. */
  private var densityDpi: Int? = null

  /** Whether this device is a television (known once the application has started). */
  val isTelevision: Boolean get() = densityDpi != null
  private var measured = false

  private fun measure(base: Context) {
    if (measured) return
    measured = true
    val television = (base.getSystemService(Context.UI_MODE_SERVICE) as? UiModeManager)?.currentModeType == Configuration.UI_MODE_TYPE_TELEVISION
    if (!television) return
    val display = (base.getSystemService(Context.DISPLAY_SERVICE) as? DisplayManager)?.getDisplay(Display.DEFAULT_DISPLAY) ?: return
    val real = DisplayMetrics()
    @Suppress("DEPRECATION") display.getRealMetrics(real)
    val width = maxOf(real.widthPixels, real.heightPixels)
    if (width > 0) densityDpi = Math.round(DisplayMetrics.DENSITY_DEFAULT * width / DESIGN_WIDTH)
  }

  /**
   * [base], with its resources measuring the screen in the design space when this is a
   * television. The application's context is wrapped first (and takes the measurement); an
   * activity's base context cannot reach the display yet, so it reuses that measurement.
   */
  fun context(base: Context): Context {
    measure(base)
    val dpi = densityDpi ?: return base
    val configuration = Configuration(base.resources.configuration)
    configuration.densityDpi = dpi
    return base.createConfigurationContext(configuration)
  }

  /**
   * Gives React Native the same measure, before it starts. React Native otherwise takes its
   * density from the display itself, which no resource configuration can change, and re-reads it
   * whenever the system font scale may have changed; on a television that re-read is turned off
   * so the design space holds for the life of the process.
   */
  fun install(application: Application) {
    if (densityDpi == null) return
    val window = application.resources.displayMetrics
    DisplayMetricsHolder.setWindowDisplayMetrics(window)
    DisplayMetricsHolder.setScreenDisplayMetrics(DisplayMetrics().apply { setTo(window) })
  }

  /** After React Native has loaded its feature flags. */
  fun keep(application: Application) {
    if (densityDpi == null) return
    // The flags React Native just loaded, with that one re-read off.
    ReactNativeFeatureFlags.dangerouslyForceOverride(
        object : ReactNativeFeatureFlagsProvider by ReactNativeFeatureFlagsOverrides_RNOSS_Stable_Android() {
          override fun enableFontScaleChangesUpdatingLayout(): Boolean = false
        })
  }
}
