# Add project specific ProGuard rules here.
-keep class mobile.** { *; }
-keep interface mobile.** { *; }
-keepclassmembers class mobile.** { *; }

# Tink crypto library (used by EncryptedSharedPreferences)
-dontwarn com.google.errorprone.annotations.**
-dontwarn javax.annotation.**
-dontwarn javax.annotation.concurrent.**
