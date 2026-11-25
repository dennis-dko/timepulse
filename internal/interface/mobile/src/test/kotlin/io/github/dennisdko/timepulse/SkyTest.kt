package io.github.dennisdko.timepulse

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * SkyTest holds the connect screen's sky to the places the browser suite's
 * TestTheSignInSkyFollowsTheClock reads off the sign-in screen at the same
 * hours: the two are one drawing, worked out twice.
 */
class SkyTest {
    @Test
    fun atOneInTheAfternoonTheSunStandsAtTheTopOfItsArcAndNoStarIsOut() {
        val sky = Sky.at(13f)

        assertTrue(sky.day)
        assertEquals(800f, sky.x, 0.1f)
        assertEquals(90f, sky.y, 0.1f)
        assertEquals(0f, sky.night, 0f)
        assertEquals(0xFF4EA8DE, sky.colour)
    }

    @Test
    fun atElevenAtNightTheMoonIsThreeTenthsAlongAndEveryStarIsOut() {
        val sky = Sky.at(23f)

        assertFalse(sky.day)
        assertEquals(353.3f, sky.x, 0.1f)
        assertEquals(239f, sky.y, 0.1f)
        assertEquals(1f, sky.night, 0f)
    }

    @Test
    fun aColourBetweenTwoOfTheTableIsMixedFromThem() {
        // Halfway from 9 to 13 o'clock: halfway from #7fb8e6 to #4ea8de.
        assertEquals(0xFF67B0E2, Sky.colourAt(11f))
        assertEquals(0xFF1B2A4E, Sky.colourAt(0f))
        assertEquals(0xFF1B2A4E, Sky.colourAt(24f))
    }
}
