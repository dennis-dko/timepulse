package io.github.dennisdko.timepulse

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.ClipOp
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.clipPath
import androidx.compose.ui.graphics.drawscope.withTransform
import kotlinx.coroutines.delay
import java.util.Calendar
import kotlin.math.PI
import kotlin.math.cos
import kotlin.math.max
import kotlin.math.min
import kotlin.math.roundToLong
import kotlin.math.sin

/**
 * Sky is the sky behind the web interface's sign-in card, worked out for an hour
 * of the day: app.js's drawTheSky, number for number, which test/sky_test.go
 * holds this copy to. The connect screen is where somebody signs in to the app,
 * and the maintainer chose on 2026-11-25 to draw that sky behind every such
 * screen. The drawing's own units are 1600 by 1000, and its arc runs from 40,870
 * over 800,90 to 1560,870.
 */
internal object Sky {
    /** The sky's colour through the day, as app.js's SKY: an hour and the colour at it. */
    val stops: List<Pair<Float, Long>> = listOf(
        0f to 0xFF1B2A4E, 5f to 0xFF3B3F8F, 6.5f to 0xFFF08A5D, 9f to 0xFF7FB8E6, 13f to 0xFF4EA8DE,
        17f to 0xFFF2A65A, 19.5f to 0xFFE2678B, 21f to 0xFF4B3F8F, 24f to 0xFF1B2A4E,
    )

    /** When the sun rises on the arc and sets, in hours: a day drawn, not a place's. */
    const val SUNRISE = 6f
    const val SUNSET = 20f

    /** How strongly the sky's colour shows, and the stars at their brightest, on a light and a dark ground. */
    const val LIGHT_STRENGTH = 0.22f
    const val DARK_STRENGTH = 0.34f
    const val LIGHT_STARS = 0.45f
    const val DARK_STARS = 0.7f

    /** The sun on a light and a dark ground, and the glow round the sun and round the moon. */
    const val LIGHT_SUN = 0xFFF6A23C
    const val DARK_SUN = 0xFFF0A44E
    const val DAY_HALO = 0xFFFFD27A
    const val NIGHT_HALO = 0xFFC9D6FF

    /** The stars, as x, y and radius, where the sign-in screen's svg has them. */
    val stars = listOf(
        Triple(150f, 120f, 3.5f), Triple(330f, 220f, 2.5f), Triple(520f, 90f, 2.5f),
        Triple(700f, 170f, 3.5f), Triple(960f, 70f, 2.5f), Triple(1120f, 190f, 2.5f),
        Triple(1290f, 110f, 3.5f), Triple(1470f, 230f, 2.5f), Triple(240f, 340f, 2.5f),
        Triple(1390f, 360f, 3.5f), Triple(610f, 300f, 2.5f), Triple(1010f, 280f, 2.5f),
        Triple(80f, 470f, 3.5f), Triple(1530f, 480f, 2.5f),
    )

    /** Six, nine, noon, three and six o'clock on the arc, unlabelled as on the sign-in screen. */
    val ticks = listOf(
        Offset(40f, 870f), Offset(205.8f, 383.7f), Offset(630.9f, 109.6f), Offset(1129.8f, 167.2f), Offset(1484.8f, 531.6f),
    )

    /** At is the sky at one hour: its colour, the sun's or the moon's place, and how far the stars are out. */
    data class At(val colour: Long, val x: Float, val y: Float, val day: Boolean, val night: Float)

    /** At is where everything stands at an hour of the day, as drawTheSky places it. */
    fun at(hour: Float): At {
        val day = hour >= SUNRISE && hour <= SUNSET
        val along = if (day) {
            (hour - SUNRISE) / (SUNSET - SUNRISE)
        } else {
            ((if (hour < SUNRISE) hour + 24 else hour) - SUNSET) / (24 - SUNSET + SUNRISE)
        }
        val angle = PI * (1 - along)
        val night = if (day) max(0f, 1 - min(hour - SUNRISE, SUNSET - hour) / 1.5f) * 0.5f else 1f

        return At(colourAt(hour), (800 + 760 * cos(angle)).toFloat(), (870 - 780 * sin(angle)).toFloat(), day, night)
    }

    /** ColourAt is the sky's colour at an hour, mixed from the two colours of the table it falls between. */
    fun colourAt(hour: Float): Long {
        for (i in 1 until stops.size) {
            val (fromHour, from) = stops[i - 1]
            val (toHour, to) = stops[i]

            if (hour <= toHour) {
                val share = (hour - fromHour) / (toHour - fromHour)
                val mix = { shift: Int ->
                    (((from shr shift) and 255L) * (1 - share) + ((to shr shift) and 255L) * share).roundToLong()
                }

                return 0xFF000000 or (mix(16) shl 16) or (mix(8) shl 8) or mix(0)
            }
        }

        return stops.first().second
    }
}

/**
 * DaySky draws [Sky] behind a screen at the hour the phone is in, again every
 * five minutes, in the drawing's units scaled to cover the screen and kept to its
 * foot - what the web page's preserveAspectRatio, xMidYMax slice, does.
 */
@Composable
fun DaySky(modifier: Modifier = Modifier) {
    val tones = LocalTones.current
    val dark = isSystemInDarkTheme()
    var hour by remember { mutableFloatStateOf(hourNow()) }

    LaunchedEffect(Unit) {
        while (true) {
            delay(5 * 60 * 1000L)
            hour = hourNow()
        }
    }

    val sky = Sky.at(hour)
    val tint = Color(sky.colour)
    val strength = if (dark) Sky.DARK_STRENGTH else Sky.LIGHT_STRENGTH
    val starlight = if (dark) Sky.DARK_STARS else Sky.LIGHT_STARS
    val sun = Color(if (dark) Sky.DARK_SUN else Sky.LIGHT_SUN)
    val halo = Color(if (sky.day) Sky.DAY_HALO else Sky.NIGHT_HALO)

    Canvas(modifier) {
        val scale = max(size.width / 1600f, size.height / 1000f)

        withTransform({
            translate((size.width - 1600f * scale) / 2, size.height - 1000f * scale)
            scale(scale, scale, pivot = Offset.Zero)
        }) {
            val whole = Size(1600f, 1000f)
            val at = Offset(sky.x, sky.y)

            drawRect(Brush.verticalGradient(0f to tint.copy(alpha = strength), 1f to tint.copy(alpha = 0f), startY = 0f, endY = 1000f), size = whole)
            drawRect(Brush.radialGradient(0f to tint.copy(alpha = strength * 1.4f), 1f to tint.copy(alpha = 0f), center = at, radius = 900f), size = whole)

            for ((x, y, r) in Sky.stars) {
                drawCircle(tones.text.copy(alpha = sky.night * starlight), r, Offset(x, y))
            }

            drawLine(tones.muted.copy(alpha = 0.25f), Offset(0f, 870f), Offset(1600f, 870f), strokeWidth = 2f)
            drawPath(
                Path().apply { arcTo(Rect(40f, 90f, 1560f, 1650f), 180f, 180f, true) },
                tones.muted.copy(alpha = 0.5f),
                style = Stroke(width = 3f, cap = StrokeCap.Round, pathEffect = PathEffect.dashPathEffect(floatArrayOf(3f, 16f))),
            )

            for (tick in Sky.ticks) drawCircle(tones.muted.copy(alpha = 0.55f), 6f, tick)

            drawCircle(Brush.radialGradient(0f to halo.copy(alpha = 0.55f), 1f to halo.copy(alpha = 0f), center = at, radius = 150f), 150f, at)

            if (sky.day) {
                drawCircle(sun, 44f, at)
            } else {
                // The crescent: the moon with a smaller disc taken out of its upper right.
                clipPath(Path().apply { addOval(Rect(Offset(sky.x + 18, sky.y - 12), 36f)) }, ClipOp.Difference) {
                    drawCircle(tones.text.copy(alpha = 0.85f), 40f, at)
                }
            }
        }
    }
}

/** HourNow is the time of day on the phone's clock, in hours. */
private fun hourNow(): Float = Calendar.getInstance().let { it.get(Calendar.HOUR_OF_DAY) + it.get(Calendar.MINUTE) / 60f }
