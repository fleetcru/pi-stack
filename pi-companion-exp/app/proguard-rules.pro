# kotlinx.serialization — keep serializable classes and their companion serializers
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.AnnotationsKt

-keepclassmembers @kotlinx.serialization.Serializable class ** {
    *** Companion;
}
-keepclasseswithmembers class **$$serializer {
    *** INSTANCE;
}

# Keep all @Serializable model classes used by the API client
-keep class com.example.picompanion.data.model.** { *; }
-keep class com.example.picompanion.data.settings.** { *; }
-keep class com.example.picompanion.data.api.PromptImage { *; }

# msgpack-core selects its platform-specific buffer implementation through
# Class.forName(). R8 cannot see those reflective references and otherwise
# removes MessageBufferU from release builds.
-keep class org.msgpack.core.buffer.MessageBufferU { *; }
-keep class org.msgpack.core.buffer.MessageBufferBE { *; }
-dontwarn sun.nio.ch.DirectBuffer

# OkHttp — suppress warnings from the platform
-dontwarn okhttp3.internal.platform.**
-dontwarn org.conscrypt.**
-dontwarn org.bouncycastle.**
-dontwarn org.openjsse.**

# Google Tink / errorprone annotations (used by EncryptedSharedPreferences)
-dontwarn com.google.errorprone.annotations.**

# Coroutines
-keepnames class kotlinx.coroutines.internal.MainDispatcherFactory {}
-keepnames class kotlinx.coroutines.CoroutineExceptionHandler {}
