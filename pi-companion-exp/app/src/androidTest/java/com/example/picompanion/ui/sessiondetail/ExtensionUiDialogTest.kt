package com.example.picompanion.ui.sessiondetail

import androidx.compose.material3.MaterialTheme
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.getUnclippedBoundsInRoot
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.unit.dp
import com.example.picompanion.data.model.ExtensionUiMethod
import com.example.picompanion.data.model.ExtensionUiOption
import com.example.picompanion.data.model.ExtensionUiRequest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ExtensionUiDialogTest {
  @get:Rule
  val composeRule = createComposeRule()

  @Test
  fun selectingOptionEnablesSubmitAndReturnsSelection() {
    var submitted: ExtensionUiAnswer? = null
    composeRule.setContent {
      MaterialTheme {
        ExtensionUiDialog(
          request = ExtensionUiRequest(
            id = "ask-1",
            method = ExtensionUiMethod.ASK_USER,
            question = "Choose an option",
            options = listOf(
              ExtensionUiOption("First", "The first choice"),
              ExtensionUiOption("Second", "The second choice"),
            ),
          ),
          submitError = null,
          submitting = false,
          onConfirm = { submitted = it },
          onCancel = {},
          onDismissLocal = {},
        )
      }
    }

    val dialogBounds = composeRule
      .onNodeWithTag("extension-ui-dialog")
      .getUnclippedBoundsInRoot()
    val dialogWidth = dialogBounds.right - dialogBounds.left
    assertTrue("dialog width $dialogWidth should be at most 380.dp", dialogWidth <= 380.dp)
    composeRule.onNodeWithText("Submit").assertIsNotEnabled()
    composeRule.onNodeWithTag("extension-ui-option-First").performClick()
    composeRule.onNodeWithText("Submit").assertIsEnabled().performClick()

    composeRule.runOnIdle {
      assertEquals(listOf("First"), submitted?.selections)
      assertEquals("selection", submitted?.responseKind)
    }
  }

  @Test
  fun emptyOptionsFallsBackToTypedAnswer() {
    var submitted: ExtensionUiAnswer? = null
    composeRule.setContent {
      MaterialTheme {
        ExtensionUiDialog(
          request = ExtensionUiRequest(
            id = "ask-empty",
            method = ExtensionUiMethod.ASK_USER,
            question = "What should we do?",
          ),
          submitError = null,
          submitting = false,
          onConfirm = { submitted = it },
          onCancel = {},
          onDismissLocal = {},
        )
      }
    }

    composeRule.onNodeWithTag("extension-ui-other-input").performTextInput("Use typed input")
    composeRule.onNodeWithTag("extension-ui-submit").assertIsEnabled().performClick()
    composeRule.runOnIdle {
      assertEquals("Use typed input", submitted?.value)
      assertEquals("freeform", submitted?.responseKind)
    }
  }

  @Test
  fun otherCardUsesTypedAnswerInsteadOfOption() {
    var submitted: ExtensionUiAnswer? = null
    composeRule.setContent {
      MaterialTheme {
        ExtensionUiDialog(
          request = ExtensionUiRequest(
            id = "ask-2",
            method = ExtensionUiMethod.ASK_USER,
            question = "Choose or write another answer",
            options = listOf(ExtensionUiOption("Existing option")),
            allowFreeform = true,
          ),
          submitError = null,
          submitting = false,
          onConfirm = { submitted = it },
          onCancel = {},
          onDismissLocal = {},
        )
      }
    }

    composeRule.onNodeWithTag("extension-ui-option-Existing option").performClick()
    composeRule.onNodeWithTag("extension-ui-other").performClick()
    composeRule.onNodeWithTag("extension-ui-submit").assertIsNotEnabled()
    composeRule.onNodeWithTag("extension-ui-other-input").performTextInput("My own answer")
    composeRule.onNodeWithTag("extension-ui-submit").assertIsEnabled().performClick()

    composeRule.runOnIdle {
      assertEquals("My own answer", submitted?.value)
      assertEquals("freeform", submitted?.responseKind)
      assertEquals(null, submitted?.selections)
    }
  }
}
