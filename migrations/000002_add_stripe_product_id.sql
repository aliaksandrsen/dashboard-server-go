-- AlterTable
ALTER TABLE "Product" ADD COLUMN "stripeProductId" TEXT;

-- CreateIndex
CREATE UNIQUE INDEX "Product_stripeProductId_key" ON "Product"("stripeProductId");
